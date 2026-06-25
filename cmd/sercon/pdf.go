// cmd/sercon/pdf.go
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// validatePDFFormat lowercases f, accepts png|jpeg|tiff (default png), and
// returns the pdftoppm flag for it ("-png" / "-jpeg" / "-tiff").
func validatePDFFormat(f string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "", "png":
		return "-png", nil
	case "jpeg", "jpg":
		return "-jpeg", nil
	case "tiff", "tif":
		return "-tiff", nil
	default:
		return "", fmt.Errorf("invalid format %q (png|jpeg|tiff)", f)
	}
}

// parsePDFPages parses "", "N", or "F-L" into 1-based first/last bounds.
// "" → (0,0) meaning "all pages". Non-positive or reversed ranges error.
func parsePDFPages(spec string) (first, last int, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, nil
	}
	if !strings.Contains(spec, "-") {
		n, e := strconv.Atoi(spec)
		if e != nil || n < 1 {
			return 0, 0, fmt.Errorf("invalid page %q (expect a positive integer)", spec)
		}
		return n, n, nil
	}
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid page range %q (expect F-L)", spec)
	}
	f, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	l, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if e1 != nil || e2 != nil || f < 1 || l < 1 || l < f {
		return 0, 0, fmt.Errorf("invalid page range %q (expect F-L with 1<=F<=L)", spec)
	}
	return f, l, nil
}

type pdfImageSpec struct {
	src, prefix, format string // format is the pdftoppm flag, e.g. "-png"
	firstPage, lastPage int
	dpi                 int
}

// buildPdfImageArgs builds the `pdftoppm` argv (deterministic order).
func buildPdfImageArgs(s pdfImageSpec) []string {
	flags := []string{s.format}
	if s.firstPage > 0 {
		flags = append(flags, "-f", strconv.Itoa(s.firstPage))
	}
	if s.lastPage > 0 {
		flags = append(flags, "-l", strconv.Itoa(s.lastPage))
	}
	if s.dpi > 0 {
		flags = append(flags, "-r", strconv.Itoa(s.dpi))
	}
	return safePathArgs(flags, s.src, s.prefix)
}

type pdfTextSpec struct {
	src, dest           string
	firstPage, lastPage int
	layout              bool
}

// buildPdfTextArgs builds the `pdftotext` argv; empty dest → stdout ("-").
func buildPdfTextArgs(s pdfTextSpec) []string {
	flags := []string{}
	if s.firstPage > 0 {
		flags = append(flags, "-f", strconv.Itoa(s.firstPage))
	}
	if s.lastPage > 0 {
		flags = append(flags, "-l", strconv.Itoa(s.lastPage))
	}
	if s.layout {
		flags = append(flags, "-layout")
	}
	dest := s.dest
	if dest == "" {
		dest = "-"
	}
	return safePathArgs(flags, s.src, dest)
}

type pdfHTMLSpec struct {
	src, dest           string
	firstPage, lastPage int
}

// buildPdfHTMLArgs builds the `pdftohtml` argv. -i ignores images (keeps the
// output a single self-contained file); -noframes emits one HTML document.
func buildPdfHTMLArgs(s pdfHTMLSpec) []string {
	flags := []string{"-i", "-noframes"}
	if s.firstPage > 0 {
		flags = append(flags, "-f", strconv.Itoa(s.firstPage))
	}
	if s.lastPage > 0 {
		flags = append(flags, "-l", strconv.Itoa(s.lastPage))
	}
	return safePathArgs(flags, s.src, s.dest)
}

// parsePdfInfo turns `pdfinfo` "Key: Value" output into the info() object.
func parsePdfInfo(out string) map[string]any {
	keyMap := map[string]string{
		"Title": "title", "Author": "author", "Creator": "creator",
		"Producer": "producer", "Page size": "pageSize", "File size": "fileSize",
		"PDF version": "pdfVersion",
	}
	info := map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "Pages":
			if n, err := strconv.Atoi(val); err == nil {
				info["pages"] = n
			}
		case "Encrypted":
			info["encrypted"] = strings.HasPrefix(strings.ToLower(val), "yes")
		case "Tagged":
			info["tagged"] = strings.EqualFold(val, "yes")
		default:
			if mapped, ok := keyMap[key]; ok && val != "" {
				info[mapped] = val
			}
		}
	}
	return info
}

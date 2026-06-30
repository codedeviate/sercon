// cmd/sercon/codec_doc.go
package main

import "strings"

// docModel is the uniform document model: the full extracted text plus a
// paragraph breakdown. Extraction is best-effort (extraction-grade).
type docModel struct {
	format     string
	text       string
	paragraphs []string
}

// splitParagraphs splits flat text into paragraphs on blank-line runs, trimming
// each block and dropping empties. Used by readers that yield only flat text
// (pdf, doc).
func splitParagraphs(text string) []string {
	var out []string
	for _, blk := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if b := strings.TrimSpace(blk); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// joinParagraphs renders paragraphs as text, blocks separated by a blank line.
func joinParagraphs(paras []string) string { return strings.Join(paras, "\n\n") }

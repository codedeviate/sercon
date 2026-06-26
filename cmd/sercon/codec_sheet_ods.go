// cmd/sercon/codec_sheet_ods.go
package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// maxODSRepeat bounds a single number-columns/rows-repeated expansion so a
// malicious or degenerate file cannot exhaust memory.
const maxODSRepeat = 1 << 16

// --- content.xml model (matched by local name; office:/table:/text: ignored) ---

type odsDoc struct {
	XMLName xml.Name   `xml:"document-content"`
	Tables  []odsTable `xml:"body>spreadsheet>table"`
}

type odsTable struct {
	Name string   `xml:"name,attr"`
	Rows []odsRow `xml:"table-row"`
}

type odsRow struct {
	Repeated int       `xml:"number-rows-repeated,attr"`
	Cells    []odsCell `xml:"table-cell"`
}

type odsCell struct {
	ValueType string    `xml:"value-type,attr"`
	Value     string    `xml:"value,attr"`
	DateValue string    `xml:"date-value,attr"`
	TimeValue string    `xml:"time-value,attr"`
	BoolValue string    `xml:"boolean-value,attr"`
	Repeated  int       `xml:"number-columns-repeated,attr"`
	Ps        []odsText `xml:"p"`
}

// odsText captures a <text:p> and its nested <text:span> runs.
type odsText struct {
	Text  string    `xml:",chardata"`
	Spans []odsText `xml:"span"`
}

func (t odsText) text() string {
	var b strings.Builder
	b.WriteString(t.Text)
	for _, s := range t.Spans {
		b.WriteString(s.text())
	}
	return b.String()
}

// odsCellValue maps a parsed cell to a typed JS primitive.
func odsCellValue(c odsCell) any {
	switch c.ValueType {
	case "float", "percentage", "currency":
		if f, err := strconv.ParseFloat(c.Value, 64); err == nil {
			return f
		}
		return odsCellText(c)
	case "boolean":
		switch c.BoolValue {
		case "true":
			return true
		case "false":
			return false
		}
		return odsCellText(c)
	case "date":
		if c.DateValue != "" {
			return c.DateValue
		}
		return odsCellText(c)
	case "time":
		if c.TimeValue != "" {
			return c.TimeValue
		}
		return odsCellText(c)
	default: // "string" or empty
		s := odsCellText(c)
		if s == "" {
			return nil
		}
		return s
	}
}

// odsCellText joins a cell's paragraphs with newlines; "" for none.
func odsCellText(c odsCell) string {
	parts := make([]string, len(c.Ps))
	for i, p := range c.Ps {
		parts[i] = p.text()
	}
	return strings.Join(parts, "\n")
}

// readODS parses ODS bytes into a sheetBook.
func readODS(data []byte) (sheetBook, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: ods: %w", err)
	}
	var content []byte
	for _, f := range zr.File {
		if f.Name == "content.xml" {
			rc, err := f.Open()
			if err != nil {
				return sheetBook{}, fmt.Errorf("codec.sheet: ods: %w", err)
			}
			content, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return sheetBook{}, fmt.Errorf("codec.sheet: ods: %w", err)
			}
			break
		}
	}
	if content == nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: ods: content.xml not found")
	}
	var doc odsDoc
	if err := xml.Unmarshal(content, &doc); err != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: ods: %w", err)
	}
	book := sheetBook{format: "ods"}
	for ti, t := range doc.Tables {
		name := t.Name
		if name == "" {
			name = fmt.Sprintf("Sheet%d", ti+1)
		}
		rows, err := odsExpandRows(t.Rows)
		if err != nil {
			return sheetBook{}, err
		}
		book.tabs = append(book.tabs, sheetTab{name: name, rows: rows})
	}
	return book, nil
}

// odsExpandRows expands row/column repeats and trims trailing blank padding.
func odsExpandRows(rows []odsRow) ([][]any, error) {
	var out [][]any
	for _, r := range rows {
		cells, err := odsExpandCells(r.Cells)
		if err != nil {
			return nil, err
		}
		reps := r.Repeated
		if reps < 1 {
			reps = 1
		}
		// A fully blank row (no cells) need not be materialized many times;
		// one copy suffices and trailing-trim removes it. Check this BEFORE the
		// cap so a real LibreOffice trailing blank pad (often
		// number-rows-repeated="1048576") never errors — it's pure padding.
		if len(cells) == 0 {
			out = append(out, nil)
			continue
		}
		// Non-blank rows are expanded fully (Repeated is ~1 for real data); cap
		// only what we actually materialize.
		if reps > maxODSRepeat {
			return nil, fmt.Errorf("codec.sheet: ods: row repeat count %d exceeds limit %d", reps, maxODSRepeat)
		}
		for i := 0; i < reps; i++ {
			out = append(out, append([]any(nil), cells...))
		}
	}
	// Trim trailing all-blank rows.
	for len(out) > 0 && rowBlank(out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out, nil
}

func rowBlank(row []any) bool {
	for _, c := range row {
		if c != nil {
			return false
		}
	}
	return true
}

// odsExpandCells expands a row's column repeats, dropping the trailing blank
// pad (everything after the last non-nil value) while preserving interior gaps.
func odsExpandCells(cells []odsCell) ([]any, error) {
	type entry struct {
		val  any
		reps int
	}
	entries := make([]entry, 0, len(cells))
	lastNonNil := -1
	for _, c := range cells {
		reps := c.Repeated
		if reps < 1 {
			reps = 1
		}
		v := odsCellValue(c)
		if v != nil {
			lastNonNil = len(entries)
		}
		entries = append(entries, entry{val: v, reps: reps})
	}
	if lastNonNil < 0 {
		return nil, nil // entirely blank row
	}
	// Cap only the entries we actually materialize ([0, lastNonNil]); a trailing
	// blank-column pad with a huge repeat is dropped, never materialized.
	var row []any
	for i := 0; i <= lastNonNil; i++ {
		if entries[i].reps > maxODSRepeat {
			return nil, fmt.Errorf("codec.sheet: ods: column repeat count %d exceeds limit %d", entries[i].reps, maxODSRepeat)
		}
		for k := 0; k < entries[i].reps; k++ {
			row = append(row, entries[i].val)
		}
	}
	return row, nil
}

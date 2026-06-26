// cmd/sercon/codec_sheet.go
package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
)

type sheetTab struct {
	name string
	rows [][]any
}

type sheetBook struct {
	format string
	tabs   []sheetTab
}

// cellToStr renders a cell value for delimited (CSV/TSV) output.
func cellToStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// readDelimited parses CSV/TSV into a single-tab book of string cells. comma is
// ',' (csv) or '\t' (tsv); name is the sheet name (file basename or "Sheet1").
func readDelimited(data []byte, comma rune, name string) (sheetBook, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = comma
	r.FieldsPerRecord = -1 // allow ragged rows
	recs, err := r.ReadAll()
	if err != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: parse: %w", err)
	}
	if name == "" {
		name = "Sheet1"
	}
	rows := make([][]any, len(recs))
	for i, rec := range recs {
		cells := make([]any, len(rec))
		for j, s := range rec {
			cells[j] = s
		}
		rows[i] = cells
	}
	format := "csv"
	if comma == '\t' {
		format = "tsv"
	}
	return sheetBook{format: format, tabs: []sheetTab{{name: name, rows: rows}}}, nil
}

// writeDelimited renders a single-tab book to CSV/TSV. >1 sheet is an error.
func writeDelimited(book sheetBook, comma rune) ([]byte, error) {
	if len(book.tabs) > 1 {
		format := "csv"
		if comma == '\t' {
			format = "tsv"
		}
		return nil, fmt.Errorf("codec.sheet.write: %s supports a single sheet, model has %d — write xlsx or pick one sheet", format, len(book.tabs))
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = comma
	if len(book.tabs) == 1 {
		for _, row := range book.tabs[0].rows {
			rec := make([]string, len(row))
			for j, c := range row {
				rec[j] = cellToStr(c)
			}
			if err := w.Write(rec); err != nil {
				return nil, fmt.Errorf("codec.sheet.write: %w", err)
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("codec.sheet.write: %w", err)
	}
	return buf.Bytes(), nil
}

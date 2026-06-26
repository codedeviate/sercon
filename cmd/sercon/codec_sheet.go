// cmd/sercon/codec_sheet.go
package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
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

// cellToJS maps an excelize cell (its string value + type) to a typed JS
// primitive: numbers → float64, bools → bool, empty → nil, else the string.
//
// Note: OOXML numeric cells carry no `t` attribute, so excelize returns
// CellTypeUnset (0) for them — not CellTypeNumber. We therefore attempt a
// float parse for both CellTypeNumber and CellTypeUnset (the OOXML default),
// falling back to the string when the parse fails (e.g. shared-string cells
// before the type can be read).
func cellToJS(val string, t excelize.CellType) any {
	if val == "" {
		return nil
	}
	switch t {
	case excelize.CellTypeNumber, excelize.CellTypeUnset:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
		return val
	case excelize.CellTypeBool:
		// excelize renders bools as "TRUE"/"FALSE".
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
		return val == "TRUE"
	default:
		return val
	}
}

func readXLSX(data []byte) (sheetBook, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()
	book := sheetBook{format: "xlsx"}
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return sheetBook{}, fmt.Errorf("codec.sheet: xlsx %q: %w", name, err)
		}
		tab := sheetTab{name: name}
		for r, row := range rows {
			cells := make([]any, len(row))
			for c, val := range row {
				cellName, cerr := excelize.CoordinatesToCellName(c+1, r+1)
				if cerr != nil {
					cells[c] = val
					continue
				}
				ct, _ := f.GetCellType(name, cellName)
				cells[c] = cellToJS(val, ct)
			}
			tab.rows = append(tab.rows, cells)
		}
		book.tabs = append(book.tabs, tab)
	}
	return book, nil
}

func writeXLSX(book sheetBook) ([]byte, error) {
	if len(book.tabs) == 0 {
		return nil, fmt.Errorf("codec.sheet.write: xlsx requires at least one sheet")
	}
	f := excelize.NewFile() // starts with a default "Sheet1"
	defer func() { _ = f.Close() }()
	for i, tab := range book.tabs {
		name := tab.name
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		if i == 0 {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return nil, fmt.Errorf("codec.sheet.write: %w", err)
			}
		} else if _, err := f.NewSheet(name); err != nil {
			return nil, fmt.Errorf("codec.sheet.write: %w", err)
		}
		for r, row := range tab.rows {
			for c, cell := range row {
				if cell == nil {
					continue
				}
				cellName, cerr := excelize.CoordinatesToCellName(c+1, r+1)
				if cerr != nil {
					return nil, fmt.Errorf("codec.sheet.write: %w", cerr)
				}
				if err := f.SetCellValue(name, cellName, cell); err != nil {
					return nil, fmt.Errorf("codec.sheet.write: %w", err)
				}
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("codec.sheet.write: %w", err)
	}
	return buf.Bytes(), nil
}

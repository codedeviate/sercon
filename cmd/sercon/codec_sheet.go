// cmd/sercon/codec_sheet.go
package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dop251/goja"
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

// sniffSheetFormat detects xlsx by the ZIP magic bytes (PK\x03\x04); everything
// else is treated as csv (tsv only via an explicit opts.format or .tsv extension).
func sniffSheetFormat(data []byte) string {
	if len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04 {
		return "xlsx"
	}
	return "csv"
}

// sheetSrcBytes reads a path string (returning its basename as the sheet name
// and its lowercased extension) or a Uint8Array (name "Sheet1", no extension).
func sheetSrcBytes(vm *goja.Runtime, arg goja.Value) (data []byte, name, ext string) {
	if s, ok := arg.Export().(string); ok {
		b, err := os.ReadFile(s) //nolint:gosec // user-provided path is intentional
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("codec.sheet.read: %w", err)))
		}
		base := filepath.Base(s)
		return b, strings.TrimSuffix(base, filepath.Ext(base)), strings.ToLower(filepath.Ext(s))
	}
	if b, ok := arg.Export().([]byte); ok {
		return b, "Sheet1", ""
	}
	panic(vm.NewTypeError("codec.sheet.read: expected a path string or Uint8Array"))
}

// bookToJS renders a sheetBook as { format, sheets:[{name, rows}] }.
func bookToJS(vm *goja.Runtime, book sheetBook) goja.Value {
	sheets := make([]any, len(book.tabs))
	for i, tab := range book.tabs {
		rows := make([]any, len(tab.rows))
		for r, row := range tab.rows {
			rows[r] = append([]any(nil), row...)
		}
		sheets[i] = map[string]any{"name": tab.name, "rows": rows}
	}
	return vm.ToValue(map[string]any{"format": book.format, "sheets": sheets})
}

// toRows converts a []any of JS rows into [][]any cells.
func toRows(vm *goja.Runtime, rowsAny []any) [][]any {
	rows := make([][]any, len(rowsAny))
	for i, ra := range rowsAny {
		cells, ok := ra.([]any)
		if !ok {
			panic(vm.NewTypeError("codec.sheet.write: each row must be an array of cells"))
		}
		for _, c := range cells {
			switch c.(type) {
			case nil, string, bool, float64, int64, int:
				// ok: a primitive Cell value (string|number|boolean|null)
			default:
				panic(vm.NewTypeError(fmt.Sprintf("codec.sheet.write: each cell must be string|number|boolean|null, got %T", c)))
			}
		}
		rows[i] = cells
	}
	return rows
}

// jsToBook parses the JS model (a { sheets:[…] } object or a bare 2D array)
// into a sheetBook. An empty model (no sheets) throws a clear error.
func jsToBook(vm *goja.Runtime, arg goja.Value) sheetBook {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(vm.NewTypeError("codec.sheet.write: model is required ({ sheets } or a 2D array)"))
	}
	exp := arg.Export()
	// Bare 2D array → single sheet.
	if rows, ok := exp.([]any); ok {
		if len(rows) == 0 {
			panic(vm.NewTypeError("codec.sheet.write: bare array model has no rows — cannot write an empty sheet"))
		}
		return sheetBook{tabs: []sheetTab{{name: "Sheet1", rows: toRows(vm, rows)}}}
	}
	m, ok := exp.(map[string]any)
	if !ok {
		panic(vm.NewTypeError("codec.sheet.write: model must be an object with sheets or a 2D array"))
	}
	sheetsRaw, present := m["sheets"]
	if !present {
		panic(vm.NewTypeError("codec.sheet.write: model.sheets is required ({ sheets:[{name?,rows}] } or a 2D array)"))
	}
	sheetsAny, ok := sheetsRaw.([]any)
	if !ok {
		panic(vm.NewTypeError("codec.sheet.write: model.sheets must be an array"))
	}
	if len(sheetsAny) == 0 {
		panic(vm.NewTypeError("codec.sheet.write: model.sheets is empty — cannot write a workbook with no sheets"))
	}
	book := sheetBook{}
	for i, sa := range sheetsAny {
		sm, ok := sa.(map[string]any)
		if !ok {
			panic(vm.NewTypeError("codec.sheet.write: each sheet must be an object { name?, rows }"))
		}
		name, _ := sm["name"].(string)
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		rowsAny, ok := sm["rows"].([]any)
		if !ok {
			panic(vm.NewTypeError("codec.sheet.write: sheet.rows must be an array of arrays"))
		}
		book.tabs = append(book.tabs, sheetTab{name: name, rows: toRows(vm, rowsAny)})
	}
	return book
}

// sheetNamespace returns the codec.sheet sub-namespace with read and write.
func sheetNamespace(vm *goja.Runtime) map[string]any {
	throwErr := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"read": func(call goja.FunctionCall) goja.Value {
			data, name, ext := sheetSrcBytes(vm, call.Argument(0))
			format := ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if fv := o.ToObject(vm).Get("format"); fv != nil && !goja.IsUndefined(fv) {
					format = strings.ToLower(fv.String())
				}
			}
			if format == "" {
				format = sniffSheetFormat(data) // PK→xlsx else csv
				if format == "csv" && ext == ".tsv" {
					format = "tsv" // honor a .tsv path extension
				}
			}
			var book sheetBook
			var err error
			switch format {
			case "xlsx":
				book, err = readXLSX(data)
			case "tsv":
				book, err = readDelimited(data, '\t', name)
			case "csv":
				book, err = readDelimited(data, ',', name)
			default:
				return throwErr(fmt.Errorf("codec.sheet.read: unsupported format %q (csv, tsv, xlsx)", format))
			}
			if err != nil {
				return throwErr(err)
			}
			return bookToJS(vm, book)
		},
		"write": func(call goja.FunctionCall) goja.Value {
			book := jsToBook(vm, call.Argument(0))
			var opts *goja.Object
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				opts = o.ToObject(vm)
			}
			format, dest := "", ""
			if opts != nil {
				if fv := opts.Get("format"); fv != nil && !goja.IsUndefined(fv) {
					format = strings.ToLower(fv.String())
				}
				if dv := opts.Get("dest"); dv != nil && !goja.IsUndefined(dv) {
					dest = dv.String()
				}
			}
			if format == "" && dest != "" {
				switch strings.ToLower(filepath.Ext(dest)) {
				case ".xlsx":
					format = "xlsx"
				case ".tsv":
					format = "tsv"
				case ".csv":
					format = "csv"
				}
			}
			var out []byte
			var err error
			switch format {
			case "xlsx":
				out, err = writeXLSX(book)
			case "tsv":
				out, err = writeDelimited(book, '\t')
			case "csv":
				out, err = writeDelimited(book, ',')
			default:
				return throwErr(fmt.Errorf("codec.sheet.write: format is required (csv, tsv, xlsx)"))
			}
			if err != nil {
				return throwErr(err)
			}
			if dest != "" {
				if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
					return throwErr(fmt.Errorf("codec.sheet.write: %w", werr))
				}
				return vm.ToValue(map[string]any{"format": format, "path": dest})
			}
			return vm.ToValue(map[string]any{"format": format, "bytes": out})
		},
	}
}

// cmd/sercon/codec_sheet_slk.go
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// splitSylkFields splits a SYLK record on ';', treating ';;' as an escaped
// literal semicolon within a field.
func splitSylkFields(line string) []string {
	var fields []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == ';' {
			if i+1 < len(line) && line[i+1] == ';' {
				cur.WriteByte(';')
				i++
				continue
			}
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(line[i])
	}
	fields = append(fields, cur.String())
	return fields
}

// sylkValue parses a SYLK K-field value into a typed cell.
func sylkValue(v string) any {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		// SYLK escapes a literal '"' inside a quoted value by doubling it.
		return strings.ReplaceAll(v[1:len(v)-1], `""`, `"`)
	}
	switch v {
	case "TRUE":
		return true
	case "FALSE":
		return false
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}

// readSYLK parses a SYLK (.slk) document into a single-sheet book. Read-only.
func readSYLK(data []byte, name string) (sheetBook, error) {
	if name == "" {
		name = "Sheet1"
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	type cell struct {
		r, c int
		v    any
	}
	var cells []cell
	// cx/cy persist across C records by design: SYLK "current cell" semantics
	// let a C record omit X/Y to reuse the previous column/row.
	cx, cy, maxR, maxC := 0, 0, 0, 0
	sawID := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		fields := splitSylkFields(line)
		switch fields[0] {
		case "ID":
			sawID = true
		case "C":
			var val any
			haveVal := false
			for _, f := range fields[1:] {
				if f == "" {
					continue
				}
				switch f[0] {
				case 'Y':
					if n, err := strconv.Atoi(f[1:]); err == nil {
						cy = n
					}
				case 'X':
					if n, err := strconv.Atoi(f[1:]); err == nil {
						cx = n
					}
				case 'K':
					val = sylkValue(f[1:])
					haveVal = true
				}
			}
			if haveVal && cy > 0 && cx > 0 {
				cells = append(cells, cell{cy, cx, val})
				if cy > maxR {
					maxR = cy
				}
				if cx > maxC {
					maxC = cx
				}
			}
		}
		// B, F, O, P, E and other records are ignored.
	}
	if err := sc.Err(); err != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: SYLK scan: %w", err)
	}
	if !sawID {
		return sheetBook{}, fmt.Errorf("codec.sheet: not a SYLK document (missing ID record)")
	}
	const maxSYLKRows, maxSYLKCols, maxSYLKCells = 1048576, 16384, 10_000_000
	if maxR > maxSYLKRows || maxC > maxSYLKCols || int64(maxR)*int64(maxC) > maxSYLKCells {
		return sheetBook{}, fmt.Errorf("codec.sheet: SYLK grid too large (%dx%d)", maxR, maxC)
	}
	rows := make([][]any, maxR)
	for i := range rows {
		rows[i] = make([]any, maxC)
	}
	for _, cl := range cells {
		rows[cl.r-1][cl.c-1] = cl.v
	}
	return sheetBook{format: "slk", tabs: []sheetTab{{name: name, rows: rows}}}, nil
}

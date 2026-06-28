// cmd/sercon/codec_sheet_dif.go
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// readDIF parses a DIF (.dif) document into a single-sheet book. Read-only.
// The header (TABLE/VECTORS/TUPLES/… 3-line chunks) is skipped to the DATA
// chunk; data items are then read in (header-line, value-line) pairs: type -1
// is special (BOT begins a tuple/row, EOD ends), type 0 numeric (value on the
// header line; V→number, TRUE/FALSE→bool, NA/ERROR→null), type 1 string.
func readDIF(data []byte, name string) (sheetBook, error) {
	if name == "" {
		name = "Sheet1"
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: DIF scan: %w", err)
	}

	// Header: 3-line chunks until the DATA chunk.
	dataStart := -1
	for i := 0; i+2 < len(lines); i += 3 {
		if strings.TrimSpace(lines[i]) == "DATA" {
			dataStart = i + 3
			break
		}
	}
	if dataStart < 0 {
		return sheetBook{}, fmt.Errorf("codec.sheet: not a DIF document (no DATA section)")
	}

	var rows [][]any
	var cur []any
	started := false
	for j := dataStart; j+1 < len(lines); j += 2 {
		head := strings.TrimSpace(lines[j])
		valLine := lines[j+1]
		comma := strings.IndexByte(head, ',')
		if comma < 0 {
			continue
		}
		typ := strings.TrimSpace(head[:comma])
		num := strings.TrimSpace(head[comma+1:])
		valTrim := strings.TrimSpace(valLine)
		switch typ {
		case "-1": // special
			switch valTrim {
			case "BOT":
				if started {
					rows = append(rows, cur)
				}
				cur = nil
				started = true
			case "EOD":
				if started {
					rows = append(rows, cur)
					started = false
				}
				return sheetBook{format: "dif", tabs: []sheetTab{{name: name, rows: rows}}}, nil
			}
		// Cells appended here before the first BOT (only possible in malformed
		// DIF) are intentionally discarded: the first BOT resets cur. Well-formed
		// DIF always opens its data with a BOT, so this is not a real data loss.
		case "0": // numeric
			switch valTrim {
			case "TRUE":
				cur = append(cur, true)
			case "FALSE":
				cur = append(cur, false)
			case "NA", "ERROR":
				cur = append(cur, nil)
			default: // "V" or other → use the numeric value
				if f, err := strconv.ParseFloat(num, 64); err == nil {
					cur = append(cur, f)
				} else {
					cur = append(cur, nil)
				}
			}
		case "1": // string
			s := strings.TrimSpace(valLine)
			if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
				s = s[1 : len(s)-1]
			}
			cur = append(cur, s)
		}
	}
	if started { // data ended without an explicit EOD
		rows = append(rows, cur)
	}
	return sheetBook{format: "dif", tabs: []sheetTab{{name: name, rows: rows}}}, nil
}

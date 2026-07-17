package main

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// consoleTableString renders data as a Node/Bun/Deno-style bordered table.
// It returns (rendered, true) for tabular input — an array or object — and
// ("", false) for anything else (a primitive), signalling the caller to fall
// back to a console.log-style line.
//
// Layout matches the other runtimes: a leading "(index)" column (array
// indices or object keys), one column per property (union of keys across the
// object/array rows, in first-seen order), and a "Values" column collecting
// rows that are primitives. Cell text uses the same formatting as
// console.log (formatValue): primitives raw, objects/arrays as compact JSON —
// so strings are NOT quoted, unlike Node's util.inspect rendering.
//
// columns, when non-nil, replaces the auto-derived property columns: the
// table shows exactly those columns in that order (an absent column renders
// empty, matching Node), with the "(index)" column always present.
func consoleTableString(vm *goja.Runtime, data goja.Value, columns []string) (string, bool) {
	if data == nil {
		return "", false
	}
	obj, ok := data.(*goja.Object)
	if !ok {
		return "", false
	}

	// Row index labels + row values, preserving order.
	var indexLabels []string
	var rowVals []goja.Value
	if obj.ClassName() == "Array" {
		n := obj.Get("length").ToInteger()
		for i := int64(0); i < n; i++ {
			key := strconv.FormatInt(i, 10)
			indexLabels = append(indexLabels, key)
			rowVals = append(rowVals, obj.Get(key))
		}
	} else {
		for _, k := range obj.Keys() {
			indexLabels = append(indexLabels, k)
			rowVals = append(rowVals, obj.Get(k))
		}
	}

	// Property columns (first-seen union) + per-row cell text, plus a Values
	// column for primitive rows.
	var propCols []string
	seen := map[string]bool{}
	hasValues := false
	cellFor := make([]map[string]string, len(rowVals))
	valueFor := make([]string, len(rowVals))
	hasValFor := make([]bool, len(rowVals))

	for i, rv := range rowVals {
		cells := map[string]string{}
		if rvObj, keys, tabular := tabularRow(rv); tabular {
			for _, k := range keys {
				if !seen[k] {
					seen[k] = true
					propCols = append(propCols, k)
				}
				cells[k] = formatValue(vm, rvObj.Get(k))
			}
		} else {
			hasValues = true
			hasValFor[i] = true
			valueFor[i] = formatValue(vm, rv)
		}
		cellFor[i] = cells
	}

	// Final data columns: the requested set (if given) or the derived
	// property columns plus a trailing "Values" column when needed.
	var dataCols []string
	if columns != nil {
		dataCols = columns
	} else {
		dataCols = append(dataCols, propCols...)
		if hasValues {
			dataCols = append(dataCols, "Values")
		}
	}
	headers := append([]string{"(index)"}, dataCols...)

	// Build the string grid.
	grid := make([][]string, len(rowVals))
	for i := range rowVals {
		row := make([]string, len(headers))
		row[0] = indexLabels[i]
		for c, col := range dataCols {
			switch {
			case col == "Values" && hasValFor[i]:
				row[c+1] = valueFor[i]
			default:
				row[c+1] = cellFor[i][col] // "" when absent
			}
		}
		grid[i] = row
	}

	// Column widths = max rune-width over the header and every cell.
	widths := make([]int, len(headers))
	for c, h := range headers {
		widths[c] = utf8.RuneCountInString(h)
	}
	for _, row := range grid {
		for c, cell := range row {
			if w := utf8.RuneCountInString(cell); w > widths[c] {
				widths[c] = w
			}
		}
	}

	lines := []string{
		tableBorder(widths, "┌", "┬", "┐"),
		tableRowLine(headers, widths),
		tableBorder(widths, "├", "┼", "┤"),
	}
	for _, row := range grid {
		lines = append(lines, tableRowLine(row, widths))
	}
	lines = append(lines, tableBorder(widths, "└", "┴", "┘"))
	return strings.Join(lines, "\n"), true
}

// tabularRow reports whether a row value contributes property columns. Plain
// objects contribute their keys; arrays contribute their numeric indices as
// keys. Everything else (primitives, functions, Date/RegExp/Error, …) is a
// Values cell.
func tabularRow(rv goja.Value) (*goja.Object, []string, bool) {
	o, ok := rv.(*goja.Object)
	if !ok {
		return nil, nil, false
	}
	switch o.ClassName() {
	case "Array":
		n := o.Get("length").ToInteger()
		keys := make([]string, 0, n)
		for i := int64(0); i < n; i++ {
			keys = append(keys, strconv.FormatInt(i, 10))
		}
		return o, keys, true
	case "Object":
		return o, o.Keys(), true
	default:
		return nil, nil, false
	}
}

// tableBorder builds a horizontal rule (each cell padded by one space on each
// side, hence width+2) joined with the supplied corner/junction runes.
func tableBorder(widths []int, left, mid, right string) string {
	segs := make([]string, len(widths))
	for i, w := range widths {
		segs[i] = strings.Repeat("─", w+2)
	}
	return left + strings.Join(segs, mid) + right
}

// tableRowLine renders one row, centering each cell within its column width
// with a single padding space on each side.
func tableRowLine(cells []string, widths []int) string {
	segs := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		segs[i] = " " + centerText(cell, widths[i]) + " "
	}
	return "│" + strings.Join(segs, "│") + "│"
}

// centerText pads s to width w, biasing the extra space to the right when the
// padding is odd (matching the other runtimes).
func centerText(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	total := w - n
	left := total / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", total-left)
}

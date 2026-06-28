// cmd/sercon/codec_sheet_xls.go
package main

import (
	"bytes"
	"fmt"

	"github.com/extrame/xls"
)

// readXLS reads a legacy BIFF8 (.xls, Excel 97–2003) workbook into a book.
// Read-only. Cells are the library's formatted strings (extraction-grade —
// extrame/xls exposes formatted text, not raw typed values). A recover guard
// turns any panic in the third-party parser into a clean error.
func readXLS(data []byte) (book sheetBook, err error) {
	defer func() {
		if r := recover(); r != nil {
			book = sheetBook{}
			err = fmt.Errorf("codec.sheet: XLS parse failed: %v", r)
		}
	}()
	wb, oerr := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if oerr != nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: XLS parse: %w", oerr)
	}
	if wb == nil {
		return sheetBook{}, fmt.Errorf("codec.sheet: XLS parse: not a valid workbook")
	}
	book = sheetBook{format: "xls"}
	for i := 0; i < wb.NumSheets(); i++ {
		sh := wb.GetSheet(i)
		if sh == nil {
			continue
		}
		tab := sheetTab{name: sh.Name}
		for r := 0; r <= int(sh.MaxRow); r++ {
			row := sh.Row(r)
			if row == nil {
				tab.rows = append(tab.rows, nil)
				continue
			}
			last := row.LastCol()
			cells := make([]any, 0, last)
			for c := 0; c < last; c++ {
				cells = append(cells, row.Col(c))
			}
			tab.rows = append(tab.rows, cells)
		}
		book.tabs = append(book.tabs, tab)
	}
	if len(book.tabs) == 0 {
		return sheetBook{}, fmt.Errorf("codec.sheet: XLS has no sheets")
	}
	return book, nil
}

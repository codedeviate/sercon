// cmd/sercon/codec_doc_doc.go
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/EndFirstCorp/doc2txt"
)

// readDOC extracts text from a legacy Word 97-2003 (.doc, OLE2/BIFF) document.
// Read-only. A recover guard converts third-party parser panics into errors.
func readDOC(data []byte) (m docModel, err error) {
	defer func() {
		if r := recover(); r != nil {
			m = docModel{}
			err = fmt.Errorf("codec.doc: DOC parse failed: %v", r)
		}
	}()
	rd, perr := doc2txt.ParseDoc(bytes.NewReader(data))
	if perr != nil {
		return docModel{}, fmt.Errorf("codec.doc: DOC parse: %w", perr)
	}
	b, rerr := io.ReadAll(rd)
	if rerr != nil {
		return docModel{}, fmt.Errorf("codec.doc: DOC read: %w", rerr)
	}
	text := string(b)
	var paras []string
	for _, ln := range strings.FieldsFunc(text, func(r rune) bool { return r == '\r' || r == '\n' }) {
		if s := strings.TrimSpace(ln); s != "" {
			paras = append(paras, s)
		}
	}
	return docModel{format: "doc", text: strings.ReplaceAll(text, "\r", "\n"), paragraphs: paras}, nil
}

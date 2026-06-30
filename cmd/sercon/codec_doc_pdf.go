// cmd/sercon/codec_doc_pdf.go
package main

import (
	"bytes"
	"fmt"

	"github.com/ledongthuc/pdf"
)

// readPDF extracts plain text from a PDF. Read-only. A recover guard converts
// any panic in the third-party parser into a clean error.
func readPDF(data []byte) (m docModel, err error) {
	defer func() {
		if r := recover(); r != nil {
			m = docModel{}
			err = fmt.Errorf("codec.doc: PDF parse failed: %v", r)
		}
	}()
	r, perr := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if perr != nil {
		return docModel{}, fmt.Errorf("codec.doc: PDF parse: %w", perr)
	}
	rd, terr := r.GetPlainText()
	if terr != nil {
		return docModel{}, fmt.Errorf("codec.doc: PDF text: %w", terr)
	}
	var buf bytes.Buffer
	if _, rerr := buf.ReadFrom(rd); rerr != nil {
		return docModel{}, fmt.Errorf("codec.doc: PDF read: %w", rerr)
	}
	text := buf.String()
	return docModel{format: "pdf", text: text, paragraphs: splitParagraphs(text)}, nil
}

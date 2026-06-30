// cmd/sercon/codec_doc_docx.go
package main

import (
	"bytes"
	"fmt"

	"github.com/fumiama/go-docx"
)

// readDOCX extracts paragraph text from a DOCX (OOXML) document.
func readDOCX(data []byte) (docModel, error) {
	doc, err := docx.Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return docModel{}, fmt.Errorf("codec.doc: DOCX parse: %w", err)
	}
	var paras []string
	for _, it := range doc.Document.Body.Items {
		// Paragraph text only for now: other item types (*docx.Table,
		// *docx.SectPr, …) are intentionally skipped in this v1 reader.
		if p, ok := it.(*docx.Paragraph); ok {
			paras = append(paras, p.String())
		}
	}
	return docModel{format: "docx", text: joinParagraphs(paras), paragraphs: paras}, nil
}

// writeDOCX builds a DOCX with one paragraph per entry.
func writeDOCX(paras []string) ([]byte, error) {
	w := docx.New().WithDefaultTheme()
	for _, p := range paras {
		w.AddParagraph().AddText(p)
	}
	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("codec.doc: DOCX write: %w", err)
	}
	return buf.Bytes(), nil
}

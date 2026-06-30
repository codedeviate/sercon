// cmd/sercon/codec_doc_odt.go
package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

const odtMimetype = "application/vnd.oasis.opendocument.text"

// readODT extracts paragraph text from an ODT package's content.xml.
func readODT(data []byte) (docModel, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return docModel{}, fmt.Errorf("codec.doc: ODT open: %w", err)
	}
	var content []byte
	for _, f := range zr.File {
		if f.Name == "content.xml" {
			rc, oerr := f.Open()
			if oerr != nil {
				return docModel{}, fmt.Errorf("codec.doc: ODT content: %w", oerr)
			}
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(rc)
			_ = rc.Close()
			content = buf.Bytes()
			break
		}
	}
	if content == nil {
		return docModel{}, fmt.Errorf("codec.doc: ODT: content.xml not found")
	}
	dec := xml.NewDecoder(bytes.NewReader(content))
	var paras []string
	var cur strings.Builder
	depth := 0
	for {
		tok, terr := dec.Token()
		if terr != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				if depth == 0 {
					cur.Reset()
				}
				depth++
			}
		case xml.CharData:
			if depth > 0 {
				cur.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "p" && depth > 0 {
				depth--
				if depth == 0 {
					paras = append(paras, cur.String())
				}
			}
		}
	}
	return docModel{format: "odt", text: joinParagraphs(paras), paragraphs: paras}, nil
}

var odtManifest = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` +
	`<manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="` + odtMimetype + `"/>` +
	`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>` +
	`</manifest:manifest>`

func odtContentXML(paras []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">`)
	b.WriteString(`<office:body><office:text>`)
	for _, p := range paras {
		b.WriteString(`<text:p>`)
		_ = xml.EscapeText(&b, []byte(p))
		b.WriteString(`</text:p>`)
	}
	b.WriteString(`</office:text></office:body></office:document-content>`)
	return b.String()
}

func odtWriteEntry(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(content))
	return err
}

// writeODT builds a minimal ODT package (mimetype + manifest + content.xml).
func writeODT(paras []string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, fmt.Errorf("codec.doc: ODT write: %w", err)
	}
	if _, err := mw.Write([]byte(odtMimetype)); err != nil {
		return nil, fmt.Errorf("codec.doc: ODT write: %w", err)
	}
	if err := odtWriteEntry(zw, "META-INF/manifest.xml", odtManifest); err != nil {
		return nil, fmt.Errorf("codec.doc: ODT write: %w", err)
	}
	if err := odtWriteEntry(zw, "content.xml", odtContentXML(paras)); err != nil {
		return nil, fmt.Errorf("codec.doc: ODT write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("codec.doc: ODT write: %w", err)
	}
	return buf.Bytes(), nil
}

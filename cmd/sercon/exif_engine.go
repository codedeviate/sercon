// cmd/sercon/exif_engine.go
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
	pngstructure "github.com/dsoprea/go-png-image-structure/v2"
)

var errNoExif = errors.New("no exif")

type writeMode int

const (
	modeMerge writeMode = iota
	modeReplace
	modeClear
)

// readExifDsoprea turns a raw EXIF (TIFF-structured) blob into an exifDoc.
func readExifDsoprea(rawExif []byte) (exifDoc, error) {
	tags, _, err := exif.GetFlatExifData(rawExif, nil)
	if err != nil {
		return nil, fmt.Errorf("parse exif: %w", err)
	}
	doc := exifDoc{}
	for _, tg := range tags {
		group, ok := ifdPathToGroup(tg.IfdPath)
		if !ok {
			continue // skip maker-note / unmapped IFDs for the named model
		}
		if doc[group] == nil {
			doc[group] = map[string]any{}
		}
		doc[group][tg.TagName] = encodeExifValue(tg.Value)
	}
	// Collapse GPS lat/long rationals → signed decimal for ergonomics.
	gpsDecimalize(doc)
	return doc, nil
}

// extractRawExif pulls the TIFF-structured EXIF blob out of a container.
func extractRawExif(data []byte, format string) ([]byte, error) {
	switch format {
	case "jpeg", "tiff":
		raw, err := exif.SearchAndExtractExif(data)
		if err != nil {
			if errors.Is(err, exif.ErrNoExif) {
				return nil, errNoExif
			}
			return nil, err
		}
		return raw, nil
	case "png":
		mc, err := pngstructure.NewPngMediaParser().ParseBytes(data)
		if err != nil {
			return nil, err
		}
		cs := mc.(*pngstructure.ChunkSlice)
		// cs.Exif() returns (rootIfd, rawExifBytes, error)
		_, rawExif, err := cs.Exif()
		if err != nil {
			if errors.Is(err, exif.ErrNoExif) {
				return nil, errNoExif
			}
			return nil, err
		}
		return rawExif, nil
	default:
		return nil, fmt.Errorf("extractRawExif: unsupported format %q", format)
	}
}

// newFreshRootIb builds a completely empty root IfdBuilder for replace/clear.
func newFreshRootIb() (*exif.IfdBuilder, error) {
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, err
	}
	ti := exif.NewTagIndex()
	return exif.NewIfdBuilder(im, ti, exifcommon.IfdStandardIfdIdentity, binary.BigEndian), nil
}

// buildRootIb returns the IfdBuilder to write: existing chain for merge,
// a fresh empty root for replace (or when the image has no EXIF).
func buildRootIb(existing *exif.IfdBuilder, mode writeMode) (*exif.IfdBuilder, error) {
	if mode == modeMerge && existing != nil {
		return existing, nil
	}
	return newFreshRootIb()
}

// tagIDForName resolves a standard tag name to its uint16 id within ib's IFD.
// Uses the shared exifTagIndex (read-only) so no per-call allocation is needed.
func tagIDForName(ib *exif.IfdBuilder, name string) (uint16, bool) {
	it, err := exifTagIndex.GetWithName(ib.IfdIdentity(), name)
	if err != nil {
		return 0, false
	}
	return it.Id, true
}

// wrapForEncoder ensures the value is in a slice form the dsoprea ValueEncoder
// accepts. SetStandardWithName passes values straight to Encode(), which only
// accepts slice types ([]uint16, []uint32, []Rational, etc.) — not scalars.
func wrapForEncoder(v any) any {
	switch t := v.(type) {
	case uint16:
		return []uint16{t}
	case uint32:
		return []uint32{t}
	case int32:
		return []int32{t}
	case exifcommon.Rational:
		return []exifcommon.Rational{t}
	case exifcommon.SignedRational:
		return []exifcommon.SignedRational{t}
	default:
		return v
	}
}

// applyDoc writes the doc's tags into rootIb. A nil value deletes the tag.
// GPS latitude/longitude arrive as float64 decimal degrees and must be
// converted to the [deg, min, sec] rational triple + Ref before writing.
func applyDoc(rootIb *exif.IfdBuilder, doc exifDoc) error {
	for group, tags := range doc {
		ifdPath, ok := groupToIfdPath(group)
		if !ok {
			return fmt.Errorf("unknown exif group %q (use image/exif/gps/thumbnail)", group)
		}
		ib, err := exif.GetOrCreateIbFromRootIb(rootIb, ifdPath)
		if err != nil {
			return err
		}
		for name, val := range tags {
			if val == nil { // null → delete
				if id, ok := tagIDForName(ib, name); ok {
					_, _ = ib.DeleteAll(id)
				}
				continue
			}

			// GPS lat/long decimal-degree special case: convert before the
			// generic decodeExifValue path (which would error on a scalar for
			// a RATIONAL tag).
			if group == "gps" && (name == "GPSLatitude" || name == "GPSLongitude") {
				deg, ok := val.(float64)
				if !ok {
					return fmt.Errorf("exif: gps.%s: expected a float64 decimal degree, got %T", name, val)
				}
				isLat := name == "GPSLatitude"
				rats, ref := decimalToGPS(deg, isLat)
				if err := ib.SetStandardWithName(name, rats); err != nil {
					return fmt.Errorf("gps.%s: %w", name, err)
				}
				refName := "GPSLatitudeRef"
				if !isLat {
					refName = "GPSLongitudeRef"
				}
				if err := ib.SetStandardWithName(refName, ref); err != nil {
					return fmt.Errorf("gps.%s: %w", refName, err)
				}
				continue
			}

			goVal, err := decodeExifValue(group, name, val)
			if err != nil {
				return err
			}
			if err := ib.SetStandardWithName(name, wrapForEncoder(goVal)); err != nil {
				return fmt.Errorf("%s.%s: %w", group, name, err)
			}
		}
	}
	return nil
}

func writeExifJPEG(data []byte, doc exifDoc, mode writeMode) ([]byte, error) {
	mc, err := jpegstructure.NewJpegMediaParser().ParseBytes(data)
	if err != nil {
		return nil, err
	}
	sl := mc.(*jpegstructure.SegmentList)

	if mode == modeClear {
		// DropExif returns (wasDropped bool, err error)
		_, dropErr := sl.DropExif()
		if dropErr != nil {
			return nil, dropErr
		}
		var b bytes.Buffer
		if err := sl.Write(&b); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}

	// ConstructExifBuilder returns an error when there is no EXIF; buildRootIb
	// handles nil existing by building a fresh root.
	existing, _ := sl.ConstructExifBuilder()
	rootIb, err := buildRootIb(existing, mode)
	if err != nil {
		return nil, err
	}
	if err := applyDoc(rootIb, doc); err != nil {
		return nil, err
	}
	if err := sl.SetExif(rootIb); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := sl.Write(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeExifPNG(data []byte, doc exifDoc, mode writeMode) ([]byte, error) {
	if mode == modeClear {
		// pngstructure has no chunk-drop API (the underlying chunks field is
		// unexported and SetExif only adds/replaces — writing an empty builder
		// leaves an eXIf chunk behind). Strip the eXIf chunk with a pure-Go
		// PNG walker instead.
		return stripPNGExif(data)
	}

	mc, err := pngstructure.NewPngMediaParser().ParseBytes(data)
	if err != nil {
		return nil, err
	}
	cs := mc.(*pngstructure.ChunkSlice)

	existing, _ := cs.ConstructExifBuilder()
	rootIb, err := buildRootIb(existing, mode)
	if err != nil {
		return nil, err
	}
	if err := applyDoc(rootIb, doc); err != nil {
		return nil, err
	}
	if err := cs.SetExif(rootIb); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := cs.WriteTo(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// stripPNGExif removes any eXIf chunk(s) from a PNG and returns the rebuilt
// bytes. A PNG is the 8-byte signature followed by chunks laid out as
// [uint32 length][4-byte type][length bytes data][uint32 CRC]. We copy every
// chunk except those whose type is "eXIf", preserving each chunk's own CRC.
func stripPNGExif(data []byte) ([]byte, error) {
	if len(data) < len(pngSignature) || !bytes.Equal(data[:len(pngSignature)], pngSignature) {
		return nil, fmt.Errorf("stripPNGExif: not a valid PNG (bad signature)")
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:len(pngSignature)]...)

	pos := len(pngSignature)
	for pos < len(data) {
		// Each chunk needs at least 4 (length) + 4 (type) + 4 (CRC) bytes.
		if pos+8 > len(data) {
			return nil, fmt.Errorf("stripPNGExif: truncated PNG (incomplete chunk header at %d)", pos)
		}
		length := binary.BigEndian.Uint32(data[pos : pos+4])
		typ := string(data[pos+4 : pos+8])
		chunkEnd := pos + 12 + int(length) // 4 len + 4 type + data + 4 CRC
		if chunkEnd > len(data) {
			return nil, fmt.Errorf("stripPNGExif: truncated PNG (chunk %q overruns buffer)", typ)
		}
		if typ != "eXIf" {
			out = append(out, data[pos:chunkEnd]...)
		}
		pos = chunkEnd
		if typ == "IEND" {
			break
		}
	}
	return out, nil
}

// gpsDecimalize replaces gps GPSLatitude/GPSLongitude rational triples with
// signed decimals (using the *Ref tags), matching the documented JSON model.
func gpsDecimalize(doc exifDoc) {
	g := doc["gps"]
	if g == nil {
		return
	}
	conv := func(coord, ref string) {
		triple, ok := g[coord].([]any)
		if !ok || len(triple) != 3 {
			return
		}
		rats := make([]exifcommon.Rational, 3)
		for i, e := range triple {
			pair, ok := e.([]any)
			if !ok || len(pair) != 2 {
				return
			}
			n, _ := pair[0].(uint32)
			d, _ := pair[1].(uint32)
			rats[i] = exifcommon.Rational{Numerator: n, Denominator: d}
		}
		refStr, _ := g[ref].(string)
		g[coord] = gpsToDecimal(rats, refStr)
	}
	conv("GPSLatitude", "GPSLatitudeRef")
	conv("GPSLongitude", "GPSLongitudeRef")
}

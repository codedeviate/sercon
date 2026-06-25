// cmd/sercon/exif_model.go
package main

import (
	"encoding/base64"
	"fmt"
	"math"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// exifDoc is the canonical grouped-by-IFD model bridging JSON and both EXIF
// backends. Keys: "image" (IFD0), "exif" (Exif sub-IFD), "gps", "thumbnail".
type exifDoc map[string]map[string]any

var ifdGroupByPath = map[string]string{
	"IFD": "image", "IFD/Exif": "exif", "IFD/GPS": "gps", "IFD1": "thumbnail",
}
var ifdPathByGroup = map[string]string{
	"image": "IFD", "exif": "IFD/Exif", "gps": "IFD/GPS", "thumbnail": "IFD1",
}

func ifdPathToGroup(ifdPath string) (string, bool) { g, ok := ifdGroupByPath[ifdPath]; return g, ok }
func groupToIfdPath(group string) (string, bool)   { p, ok := ifdPathByGroup[group]; return p, ok }

// encodeExifValue converts a dsoprea-decoded value into a JSON-friendly form.
func encodeExifValue(v any) any {
	switch t := v.(type) {
	case exifcommon.Rational:
		return []any{t.Numerator, t.Denominator}
	case exifcommon.SignedRational:
		return []any{t.Numerator, t.Denominator}
	case []exifcommon.Rational:
		out := make([]any, len(t))
		for i, r := range t {
			out[i] = []any{r.Numerator, r.Denominator}
		}
		return out
	case []exifcommon.SignedRational:
		out := make([]any, len(t))
		for i, r := range t {
			out[i] = []any{r.Numerator, r.Denominator}
		}
		return out
	case []byte:
		return base64.StdEncoding.EncodeToString(t)
	default:
		return v // string / numeric / []string etc. pass through
	}
}

// exifTagIndex is the dsoprea standard-tag registry; it is read-only after
// construction so a single shared instance is safe (no per-Run state).
var exifTagIndex = exif.NewTagIndex()

// groupToIfdIdentity maps a doc group to the dsoprea IFD identity used to look
// up a tag's type in the standard registry.
var groupToIfdIdentity = map[string]*exifcommon.IfdIdentity{
	"image":     exifcommon.IfdStandardIfdIdentity,
	"exif":      exifcommon.IfdExifStandardIfdIdentity,
	"gps":       exifcommon.IfdGpsInfoStandardIfdIdentity,
	"thumbnail": exifcommon.Ifd1StandardIfdIdentity,
}

// decodeExifValue converts a JSON value back into the Go type that
// IfdBuilder.SetStandardWithName expects for the given tag. It is type-aware:
// the tag's primary EXIF type (resolved from the dsoprea standard registry)
// decides the coercion, so a [n,d] array becomes the right kind of rational and
// a base64 string becomes []byte. No silent coercion — a mismatched value shape
// or an unknown tag is a hard error.
func decodeExifValue(group, tagName string, jsonVal any) (any, error) {
	ii, ok := groupToIfdIdentity[group]
	if !ok {
		return nil, fmt.Errorf("exif: unknown group %q", group)
	}
	it, err := exifTagIndex.GetWithName(ii, tagName)
	if err != nil {
		return nil, fmt.Errorf("exif: %s.%s: unknown tag", group, tagName)
	}
	if len(it.SupportedTypes) == 0 {
		return nil, fmt.Errorf("exif: %s.%s: no supported type", group, tagName)
	}
	typ := it.SupportedTypes[0]

	switch typ {
	case exifcommon.TypeAscii:
		s, ok := jsonVal.(string)
		if !ok {
			return nil, fmt.Errorf("exif: %s.%s: expected a string", group, tagName)
		}
		return s, nil

	case exifcommon.TypeByte, exifcommon.TypeUndefined:
		s, ok := jsonVal.(string)
		if !ok {
			return nil, fmt.Errorf("exif: %s.%s: expected a base64 string", group, tagName)
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("exif: %s.%s: invalid base64: %w", group, tagName, err)
		}
		return b, nil

	case exifcommon.TypeShort:
		if arr, ok := jsonVal.([]any); ok {
			out := make([]uint16, len(arr))
			for i, e := range arr {
				u, uok := toUint32(e)
				if !uok || u > math.MaxUint16 {
					return nil, fmt.Errorf("exif: %s.%s: expected unsigned 16-bit numbers", group, tagName)
				}
				out[i] = uint16(u)
			}
			return out, nil
		}
		u, uok := toUint32(jsonVal)
		if !uok || u > math.MaxUint16 {
			return nil, fmt.Errorf("exif: %s.%s: expected an unsigned 16-bit number", group, tagName)
		}
		return uint16(u), nil

	case exifcommon.TypeLong:
		if arr, ok := jsonVal.([]any); ok {
			out := make([]uint32, len(arr))
			for i, e := range arr {
				u, uok := toUint32(e)
				if !uok {
					return nil, fmt.Errorf("exif: %s.%s: expected unsigned 32-bit numbers", group, tagName)
				}
				out[i] = u
			}
			return out, nil
		}
		u, uok := toUint32(jsonVal)
		if !uok {
			return nil, fmt.Errorf("exif: %s.%s: expected an unsigned 32-bit number", group, tagName)
		}
		return u, nil

	case exifcommon.TypeSignedLong:
		n, nok := toInt32(jsonVal)
		if !nok {
			return nil, fmt.Errorf("exif: %s.%s: expected a signed 32-bit number", group, tagName)
		}
		return n, nil

	case exifcommon.TypeRational:
		pair, ok := jsonVal.([]any)
		if !ok || len(pair) != 2 {
			return nil, fmt.Errorf("exif: %s.%s: expected a [numerator, denominator] array", group, tagName)
		}
		n, nok := toUint32(pair[0])
		d, dok := toUint32(pair[1])
		if !nok || !dok {
			return nil, fmt.Errorf("exif: %s.%s: rational parts must be unsigned numbers", group, tagName)
		}
		return exifcommon.Rational{Numerator: n, Denominator: d}, nil

	case exifcommon.TypeSignedRational:
		pair, ok := jsonVal.([]any)
		if !ok || len(pair) != 2 {
			return nil, fmt.Errorf("exif: %s.%s: expected a [numerator, denominator] array", group, tagName)
		}
		n, nok := toInt32(pair[0])
		d, dok := toInt32(pair[1])
		if !nok || !dok {
			return nil, fmt.Errorf("exif: %s.%s: signed-rational parts must be numbers", group, tagName)
		}
		return exifcommon.SignedRational{Numerator: n, Denominator: d}, nil

	default:
		return nil, fmt.Errorf("exif: %s.%s: unsupported tag type %v", group, tagName, typ)
	}
}

func toUint32(v any) (uint32, bool) {
	if f, ok := v.(float64); ok && f >= 0 && f <= math.MaxUint32 {
		return uint32(f), true
	}
	if u, ok := v.(uint32); ok {
		return u, true
	}
	return 0, false
}

func toInt32(v any) (int32, bool) {
	if f, ok := v.(float64); ok && f >= math.MinInt32 && f <= math.MaxInt32 {
		return int32(f), true
	}
	if i, ok := v.(int32); ok {
		return i, true
	}
	return 0, false
}

// gpsToDecimal converts the EXIF [deg,min,sec] rationals + ref to signed decimal.
func gpsToDecimal(rats []exifcommon.Rational, ref string) float64 {
	if len(rats) != 3 {
		return 0
	}
	d := float64(rats[0].Numerator) / float64(rats[0].Denominator)
	m := float64(rats[1].Numerator) / float64(rats[1].Denominator)
	s := float64(rats[2].Numerator) / float64(rats[2].Denominator)
	val := d + m/60 + s/3600
	if ref == "S" || ref == "W" {
		val = -val
	}
	return val
}

// decimalToGPS converts a signed decimal degree to the EXIF [deg,min,sec]
// rationals + N/S/E/W ref. isLat picks the N/S vs E/W ref set.
func decimalToGPS(deg float64, isLat bool) ([]exifcommon.Rational, string) {
	ref := "E"
	if isLat {
		ref = "N"
	}
	if deg < 0 {
		deg = -deg
		if isLat {
			ref = "S"
		} else {
			ref = "W"
		}
	}
	d := math.Floor(deg)
	mFull := (deg - d) * 60
	m := math.Floor(mFull)
	s := (mFull - m) * 60
	return []exifcommon.Rational{
		{Numerator: uint32(d), Denominator: 1},
		{Numerator: uint32(m), Denominator: 1},
		{Numerator: uint32(s * 1000), Denominator: 1000},
	}, ref
}

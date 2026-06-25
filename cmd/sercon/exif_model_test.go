// cmd/sercon/exif_model_test.go
package main

import (
	"reflect"
	"testing"

	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

func TestIfdPathToGroup(t *testing.T) {
	cases := map[string]string{"IFD": "image", "IFD/Exif": "exif", "IFD/GPSInfo": "gps", "IFD1": "thumbnail"}
	for path, want := range cases {
		got, ok := ifdPathToGroup(path)
		if !ok || got != want {
			t.Fatalf("ifdPathToGroup(%q)=%q,%v want %q", path, got, ok, want)
		}
	}
	if _, ok := ifdPathToGroup("IFD/MakerNote"); ok {
		t.Fatal("unknown IfdPath should map to ok=false")
	}
}

func TestGroupToIfdPath(t *testing.T) {
	if p, ok := groupToIfdPath("gps"); !ok || p != "IFD/GPSInfo" {
		t.Fatalf("groupToIfdPath(gps)=%q,%v want IFD/GPSInfo,true", p, ok)
	}
	if p, ok := groupToIfdPath("nope"); ok || p != "" {
		t.Fatalf("groupToIfdPath(nope)=%q,%v want \"\",false", p, ok)
	}
}

func TestEncodeExifValue(t *testing.T) {
	// RATIONAL → [num, den]
	if got := encodeExifValue(exifcommon.Rational{Numerator: 1, Denominator: 250}); !reflect.DeepEqual(got, []any{uint32(1), uint32(250)}) {
		t.Fatalf("rational encode = %v", got)
	}
	// []Rational (multi) → [[..],[..]]
	rs := []exifcommon.Rational{{Numerator: 1, Denominator: 2}, {Numerator: 3, Denominator: 4}}
	if got := encodeExifValue(rs); !reflect.DeepEqual(got, []any{[]any{uint32(1), uint32(2)}, []any{uint32(3), uint32(4)}}) {
		t.Fatalf("rational slice encode = %v", got)
	}
	// string passthrough
	if got := encodeExifValue("Canon"); got != "Canon" {
		t.Fatalf("string encode = %v", got)
	}
	// []byte → base64
	if got := encodeExifValue([]byte{0xDE, 0xAD}); got != "3q0=" {
		t.Fatalf("bytes encode = %v want base64 3q0=", got)
	}
}

func TestSignedRationalEncode(t *testing.T) {
	// SRATIONAL → [num, den]
	if got := encodeExifValue(exifcommon.SignedRational{Numerator: -1, Denominator: 3}); !reflect.DeepEqual(got, []any{int32(-1), int32(3)}) {
		t.Fatalf("signed rational encode = %v", got)
	}
	// []SRATIONAL (multi) → [[..],[..]]
	rs := []exifcommon.SignedRational{{Numerator: -1, Denominator: 2}, {Numerator: 3, Denominator: -4}}
	if got := encodeExifValue(rs); !reflect.DeepEqual(got, []any{[]any{int32(-1), int32(2)}, []any{int32(3), int32(-4)}}) {
		t.Fatalf("signed rational slice encode = %v", got)
	}
}

func TestDecodeExifValue_Rational(t *testing.T) {
	got, err := decodeExifValue("exif", "ExposureTime", []any{float64(1), float64(250)})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := got.(exifcommon.Rational); !ok || r.Numerator != 1 || r.Denominator != 250 {
		t.Fatalf("decode rational = %#v", got)
	}
}

func TestDecodeExifValue_TypeAware(t *testing.T) {
	// ASCII tag → string
	if got, err := decodeExifValue("image", "Make", "Canon"); err != nil || got != "Canon" {
		t.Fatalf("ascii decode = %#v err=%v", got, err)
	}
	// SHORT tag → uint16
	if got, err := decodeExifValue("image", "Orientation", float64(6)); err != nil {
		t.Fatalf("short decode err=%v", err)
	} else if u, ok := got.(uint16); !ok || u != 6 {
		t.Fatalf("short decode = %#v want uint16(6)", got)
	}
	// RATIONAL tag → exifcommon.Rational
	if got, err := decodeExifValue("exif", "ExposureTime", []any{float64(1), float64(250)}); err != nil {
		t.Fatalf("rational decode err=%v", err)
	} else if r, ok := got.(exifcommon.Rational); !ok || r.Numerator != 1 || r.Denominator != 250 {
		t.Fatalf("rational decode = %#v", got)
	}
	// SRATIONAL tag → exifcommon.SignedRational
	if got, err := decodeExifValue("exif", "ExposureBiasValue", []any{float64(-1), float64(3)}); err != nil {
		t.Fatalf("signed rational decode err=%v", err)
	} else if r, ok := got.(exifcommon.SignedRational); !ok || r.Numerator != -1 || r.Denominator != 3 {
		t.Fatalf("signed rational decode = %#v", got)
	}
	// UNDEFINED/byte tag → base64-decoded []byte ("3q0=" = 0xDE,0xAD)
	if got, err := decodeExifValue("exif", "ComponentsConfiguration", "3q0="); err != nil {
		t.Fatalf("undefined decode err=%v", err)
	} else if b, ok := got.([]byte); !ok || !reflect.DeepEqual(b, []byte{0xDE, 0xAD}) {
		t.Fatalf("undefined decode = %#v want []byte{0xDE,0xAD}", got)
	}
}

func TestDecodeExifValue_UnknownTag(t *testing.T) {
	if _, err := decodeExifValue("gps", "TotallyNotATag", float64(1)); err == nil {
		t.Fatal("unknown tag should return an error")
	}
}

func TestGPSDecimalRoundTrip(t *testing.T) {
	rats, ref := decimalToGPS(57.7089, false)
	back := gpsToDecimal(rats, ref)
	if d := back - 57.7089; d > 1e-4 || d < -1e-4 {
		t.Fatalf("gps round-trip drifted: got %v", back)
	}
}

func TestGPSDecimalRoundTrip_NegativeLat(t *testing.T) {
	rats, ref := decimalToGPS(-33.8688, true)
	if ref != "S" {
		t.Fatalf("decimalToGPS(-33.8688, true) ref = %q want S", ref)
	}
	back := gpsToDecimal(rats, ref)
	if d := back - -33.8688; d > 1e-4 || d < -1e-4 {
		t.Fatalf("negative gps round-trip drifted: got %v", back)
	}
}

// cmd/sercon/image_stego_analysis_test.go
package main

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"
)

func encodePNGForTest(t *testing.T, img *image.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// gradientRGBA builds a deterministic smooth-gradient image: low LSB randomness,
// so all stego signals should read LOW.
func gradientRGBA(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Pix[i] = uint8((x * 256) / w)
			img.Pix[i+1] = uint8((y * 256) / h)
			img.Pix[i+2] = uint8(((x + y) * 256) / (w + h))
			img.Pix[i+3] = 255
		}
	}
	return img
}

// embeddedRGBA takes a gradient and overwrites every R,G,B LSB with a
// deterministic pseudo-random bit (an LCG), simulating full LSB embedding:
// stego signals should read HIGH.
func embeddedRGBA(w, h int) *image.RGBA {
	img := gradientRGBA(w, h)
	var st uint32 = 0x1234567
	next := func() uint8 { st = st*1664525 + 1013904223; return uint8((st >> 24) & 1) }
	for p := 0; p < len(img.Pix)/4; p++ {
		for c := 0; c < 3; c++ {
			idx := p*4 + c
			img.Pix[idx] = (img.Pix[idx] &^ 1) | next()
		}
	}
	return img
}

func TestGammp_KnownValues(t *testing.T) {
	// Chi-square CDF with k dof at x is gammp(k/2, x/2).
	// For k=2, CDF(x)=1-exp(-x/2): at x=2 -> 1-e^-1 ≈ 0.6321.
	got := gammp(1.0, 1.0) // a=k/2=1, x=x/2=1
	if math.Abs(got-0.6321) > 1e-3 {
		t.Fatalf("gammp(1,1)=%.5f, want ~0.6321", got)
	}
	// CDF(0)=0, large x -> ~1.
	if gammp(1.0, 0) != 0 {
		t.Fatalf("gammp(a,0) must be 0")
	}
	if g := gammp(2.0, 50.0); g < 0.999 {
		t.Fatalf("gammp(2,50)=%.5f, want ~1", g)
	}
}

func TestLSBEntropy_LowVsHigh(t *testing.T) {
	clean := channelValues(gradientRGBA(128, 128), 0)
	emb := channelValues(embeddedRGBA(128, 128), 0)
	ce, ee := lsbEntropy(clean), lsbEntropy(emb)
	if ee < 0.99 {
		t.Errorf("embedded LSB entropy = %.4f, want ~1.0", ee)
	}
	if ce >= ee {
		t.Errorf("clean entropy (%.4f) should be below embedded (%.4f)", ce, ee)
	}
}

func TestChiSquareLSB_LowVsHigh(t *testing.T) {
	clean := channelValues(gradientRGBA(128, 128), 0)
	emb := channelValues(embeddedRGBA(128, 128), 0)
	cc, ec := chiSquareLSB(clean), chiSquareLSB(emb)
	if ec < 0.5 {
		t.Errorf("embedded chi-square prob = %.4f, want high (>0.5)", ec)
	}
	if cc > 0.5 {
		t.Errorf("clean chi-square prob = %.4f, want low (<0.5)", cc)
	}
}

func TestRSAnalysis_LowVsHigh(t *testing.T) {
	cr := rsAnalysis(gradientRGBA(128, 128), 0)
	er := rsAnalysis(embeddedRGBA(128, 128), 0)
	if cr < 0 || cr > 1 || er < 0 || er > 1 {
		t.Fatalf("rs estimates out of [0,1]: clean=%.3f emb=%.3f", cr, er)
	}
	if er <= cr {
		t.Errorf("embedded RS estimate (%.3f) should exceed clean (%.3f)", er, cr)
	}
	if cr > 0.15 {
		t.Errorf("clean RS estimate = %.3f, want near 0", cr)
	}
}

func TestStegoInspect_SerconDetected(t *testing.T) {
	// Build a real PNG carrier, embed a payload, inspect the bytes.
	carrier := stegoCarrierPNG(t, 64, 64) // helper from image_stego_test.go
	stego, err := stegoEmbed(carrier, []byte("secret message"), true, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	insp, err := stegoInspect(stego)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.serconPresent {
		t.Fatal("should detect the sercon header")
	}
	if !insp.suspicious || insp.confidence < 0.99 {
		t.Errorf("sercon payload must read suspicious with high confidence; got %v/%.2f", insp.suspicious, insp.confidence)
	}
	if insp.payloadBytes == 0 {
		t.Errorf("payloadBytes should be set")
	}
}

func TestStegoInspect_CleanNotSuspicious(t *testing.T) {
	// Encode a clean gradient to PNG and inspect.
	clean := encodePNGForTest(t, gradientRGBA(96, 96))
	insp, err := stegoInspect(clean)
	if err != nil {
		t.Fatal(err)
	}
	if insp.serconPresent {
		t.Fatal("clean image must not report a sercon header")
	}
	if insp.suspicious {
		t.Errorf("clean gradient should not be flagged suspicious; reasons=%v", insp.reasons)
	}
}

func TestStegoAnalyze_Shape(t *testing.T) {
	clean := encodePNGForTest(t, gradientRGBA(64, 64))
	m, err := stegoAnalyze(clean)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["channels"].([]any); !ok {
		t.Fatalf("analyze must return channels []any; got %T", m["channels"])
	}
	if _, ok := m["verdict"].(map[string]any); !ok {
		t.Fatalf("analyze must return verdict map")
	}
}

func TestStegoBitplane_AllSetWhite(t *testing.T) {
	// Carrier with every R LSB set to 1 → plane-0 single-channel output all white.
	img := gradientRGBA(32, 32)
	for p := 0; p < len(img.Pix)/4; p++ {
		img.Pix[p*4] |= 1 // set R LSB
	}
	out, err := stegoBitplane(encodePNGForTest(t, img), "r", 0)
	if err != nil {
		t.Fatal(err)
	}
	dec, _, err := decodeImage(out)
	if err != nil {
		t.Fatal(err)
	}
	rgba := toRGBA(dec)
	for p := 0; p < len(rgba.Pix)/4; p++ {
		if rgba.Pix[p*4] != 255 || rgba.Pix[p*4+1] != 255 || rgba.Pix[p*4+2] != 255 {
			t.Fatalf("pixel %d not white: %v", p, rgba.Pix[p*4:p*4+4])
		}
	}
}

func TestStegoBitplane_Dimensions(t *testing.T) {
	out, err := stegoBitplane(encodePNGForTest(t, gradientRGBA(40, 24)), "rgb", 3)
	if err != nil {
		t.Fatal(err)
	}
	dec, _, err := decodeImage(out)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Bounds().Dx() != 40 || dec.Bounds().Dy() != 24 {
		t.Fatalf("bitplane dims = %v, want 40x24", dec.Bounds())
	}
}

// TestStegoBitplane_RGBIsolation locks the "rgb" composite channel→colour
// mapping: an R-bit set (G/B clear) must yield pure red, catching any
// channel-swap regression.
func TestStegoBitplane_RGBIsolation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Pix = []byte{1, 0, 0, 255} // R LSB set, G/B LSB clear
	out, err := stegoBitplane(encodePNGForTest(t, img), "rgb", 0)
	if err != nil {
		t.Fatal(err)
	}
	dec, _, err := decodeImage(out)
	if err != nil {
		t.Fatal(err)
	}
	rgba := toRGBA(dec)
	if got := rgba.Pix[0:4]; got[0] != 255 || got[1] != 0 || got[2] != 0 || got[3] != 255 {
		t.Fatalf("rgb plane-0 pixel = %v, want {255,0,0,255} (R-bit -> red only)", got)
	}
}

// embeddedNRGBA overwrites the low n bits of every R,G,B channel with
// deterministic pseudo-random bits, simulating full n-bit LSB embedding.
func embeddedNRGBA(w, h, n int) *image.RGBA {
	img := gradientRGBA(w, h)
	var st uint32 = 0x1234567
	mask := byte((1 << n) - 1)
	next := func() byte { st = st*1664525 + 1013904223; return byte(st>>24) & mask }
	for p := 0; p < len(img.Pix)/4; p++ {
		for c := 0; c < 3; c++ {
			idx := p*4 + c
			img.Pix[idx] = (img.Pix[idx] &^ mask) | next()
		}
	}
	return img
}

func TestChiSquareGroups_TracksDepth(t *testing.T) {
	// On a clean gradient every depth reads low.
	clean := channelValues(gradientRGBA(128, 128), 0)
	for n := 1; n <= 4; n++ {
		if g := chiSquareGroups(clean, n); g > 0.5 {
			t.Errorf("clean chiSquareGroups n=%d = %.3f, want low", n, g)
		}
	}
	// On a 3-bit-embedded image, depths 1..3 read high.
	// Note: n=4 is not asserted — equalization of 8-value subgroups also
	// equalizes 16-value groups, so n=4 can remain high for 3-bit embeds;
	// the estimatedBits logic relies on per-channel means over a PNG-decoded
	// image where the effect dissipates, not on single-channel raw pixels.
	emb := channelValues(embeddedNRGBA(128, 128, 3), 0)
	for n := 1; n <= 3; n++ {
		if g := chiSquareGroups(emb, n); g < 0.5 {
			t.Errorf("3-bit emb chiSquareGroups n=%d = %.3f, want high", n, g)
		}
	}
}

func TestEntropyPlane_RisesOnEmbeddedPlanes(t *testing.T) {
	clean := channelValues(gradientRGBA(128, 128), 0)
	emb := channelValues(embeddedNRGBA(128, 128, 3), 0)
	// Planes 0..2 are randomized in the 3-bit embed → entropy ~1; plane 3 is not.
	for plane := 0; plane <= 2; plane++ {
		if e := entropyPlane(emb, plane); e < 0.99 {
			t.Errorf("emb entropyPlane(%d) = %.4f, want ~1", plane, e)
		}
	}
	if entropyPlane(clean, 0) >= entropyPlane(emb, 0) {
		t.Errorf("clean plane-0 entropy should be below embedded")
	}
}

func TestStegoInspect_EstimatedBits(t *testing.T) {
	clean := encodePNGForTest(t, gradientRGBA(96, 96))
	insp, err := stegoInspect(clean)
	if err != nil {
		t.Fatal(err)
	}
	if insp.estimatedBits != 0 {
		t.Errorf("clean image estimatedBits = %d, want 0", insp.estimatedBits)
	}
	stego := encodePNGForTest(t, embeddedNRGBA(96, 96, 3))
	insp3, err := stegoInspect(stego)
	if err != nil {
		t.Fatal(err)
	}
	if insp3.estimatedBits != 3 {
		t.Errorf("3-bit embed estimatedBits = %d, want 3", insp3.estimatedBits)
	}
}

func TestStegoInspect_HeaderBits(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	stego, _ := stegoEmbed(carrier, []byte("hi"), true, "", 4)
	insp, err := stegoInspect(stego)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.serconPresent || insp.headerBits != 4 {
		t.Errorf("headerBits = %d (sercon=%v), want 4", insp.headerBits, insp.serconPresent)
	}
}

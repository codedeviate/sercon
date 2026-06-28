// cmd/sercon/image_stego_analysis.go
package main

import (
	"fmt"
	"image"
	"math"
)

// channelValues extracts one channel (0=R,1=G,2=B) of rgba into a per-pixel
// byte slice (alpha excluded), in row-major order.
func channelValues(rgba *image.RGBA, c int) []byte {
	n := len(rgba.Pix) / 4
	out := make([]byte, n)
	for p := 0; p < n; p++ {
		out[p] = rgba.Pix[p*4+c]
	}
	return out
}

// entropyPlane is the Shannon entropy (bits) of bit `plane` across vals:
// 0 when all those bits are identical, 1.0 when perfectly balanced.
func entropyPlane(vals []byte, plane int) float64 {
	if len(vals) == 0 {
		return 0
	}
	ones := 0
	for _, v := range vals {
		ones += int((v >> plane) & 1)
	}
	p1 := float64(ones) / float64(len(vals))
	p0 := 1 - p1
	if p0 == 0 || p1 == 0 {
		return 0
	}
	return -(p0*math.Log2(p0) + p1*math.Log2(p1))
}

// lsbEntropy is entropyPlane for the least-significant bit (plane 0).
func lsbEntropy(vals []byte) float64 { return entropyPlane(vals, 0) }

// gammp is the regularized lower incomplete gamma P(a,x) (Numerical Recipes):
// series for x<a+1, continued fraction otherwise. Range [0,1].
func gammp(a, x float64) float64 {
	if x <= 0 || a <= 0 {
		return 0
	}
	if x < a+1 {
		return gser(a, x)
	}
	return 1 - gcf(a, x)
}

func gser(a, x float64) float64 {
	lg, _ := math.Lgamma(a)
	ap := a
	sum := 1.0 / a
	del := sum
	for i := 0; i < 300; i++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*1e-14 {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-lg)
}

func gcf(a, x float64) float64 {
	lg, _ := math.Lgamma(a)
	const tiny = 1e-30
	b := x + 1 - a
	if math.Abs(b) < tiny {
		b = tiny
	}
	c := 1.0 / tiny
	d := 1.0 / b
	h := d
	for i := 1; i < 300; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-14 {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lg) * h
}

// chiSquareLSB runs the Westfeld pairs-of-values test on a channel's byte
// values and returns the probability of LSB embedding: near 1.0 means the
// value pairs (2i,2i+1) are equalized (the embedding fingerprint), near 0
// means a natural distribution. = 1 - chiSquareCDF(chi2, dof).
func chiSquareLSB(vals []byte) float64 {
	var hist [256]int
	for _, v := range vals {
		hist[v]++
	}
	var chi2 float64
	used := 0
	for i := 0; i < 128; i++ {
		a := hist[2*i]
		b := hist[2*i+1]
		expected := float64(a+b) / 2.0
		if expected == 0 {
			continue
		}
		diff := float64(a) - expected
		chi2 += 2 * diff * diff / expected
		used++
	}
	dof := used - 1
	if dof < 1 {
		return 0
	}
	// chiSquareCDF = gammp(dof/2, chi2/2); embedding prob = 1 - CDF.
	return 1 - gammp(float64(dof)/2, chi2/2)
}

// chiSquareGroups generalizes the Westfeld pairs test to aligned groups of
// 2^n values: it returns the probability that values within each group have
// been equalized — the fingerprint of n-bit LSB replacement. n=1 is the
// classic pairs-of-values test. Range [0,1].
func chiSquareGroups(vals []byte, n int) float64 {
	size := 1 << n
	var hist [256]int
	for _, v := range vals {
		hist[v]++
	}
	var chi2 float64
	dof := 0
	for base := 0; base < 256; base += size {
		sum := 0
		for k := 0; k < size; k++ {
			sum += hist[base+k]
		}
		if sum == 0 {
			continue
		}
		expected := float64(sum) / float64(size)
		for k := 0; k < size; k++ {
			diff := float64(hist[base+k]) - expected
			chi2 += diff * diff / expected
		}
		dof += size - 1
	}
	if dof < 1 {
		return 0
	}
	return 1 - gammp(float64(dof)/2, chi2/2)
}

// --- RS analysis (Fridrich–Goljan), estimates the LSB embedding rate [0,1]. ---

// discrim is the RS discrimination function: sum of |adjacent differences|.
func discrim(g []byte) float64 {
	var s float64
	for i := 0; i+1 < len(g); i++ {
		s += math.Abs(float64(int(g[i+1]) - int(g[i])))
	}
	return s
}

// applyMask flips each sample of g per the mask: +1 -> F1 (v^1); -1 -> F_-1
// ((v+1)^1 - 1); 0 -> identity. Returns a new slice.
func applyMask(g []byte, mask []int) []byte {
	out := make([]byte, len(g))
	for i, v := range g {
		switch mask[i] {
		case 1:
			out[i] = v ^ 1
		case -1:
			out[i] = byte((int(v)+1)^1) - 1
		default:
			out[i] = v
		}
	}
	return out
}

// rsCounts returns the regular and singular group fractions for vals under
// mask, using non-overlapping groups of 4.
func rsCounts(vals []byte, mask []int) (r, s float64) {
	groups := len(vals) / 4
	if groups == 0 {
		return 0, 0
	}
	var nr, ns int
	for gi := 0; gi < groups; gi++ {
		grp := vals[gi*4 : gi*4+4]
		f0 := discrim(grp)
		f1 := discrim(applyMask(grp, mask))
		if f1 > f0 {
			nr++
		} else if f1 < f0 {
			ns++
		}
	}
	return float64(nr) / float64(groups), float64(ns) / float64(groups)
}

// rsAnalysis estimates the embedding rate of channel c via the RS quadratic.
func rsAnalysis(rgba *image.RGBA, c int) float64 {
	vals := channelValues(rgba, c)
	if len(vals) < 8 {
		return 0
	}
	mask := []int{0, 1, 1, 0}
	negMask := []int{0, -1, -1, 0}

	flipped := make([]byte, len(vals))
	for i, v := range vals {
		flipped[i] = v ^ 1
	}

	rm0, sm0 := rsCounts(vals, mask)
	rmN0, smN0 := rsCounts(vals, negMask)
	rm1, sm1 := rsCounts(flipped, mask)
	rmN1, smN1 := rsCounts(flipped, negMask)

	d0 := rm0 - sm0
	d1 := rm1 - sm1
	dn0 := rmN0 - smN0
	dn1 := rmN1 - smN1

	// Fridrich RS quadratic: 2(d1+d0)x^2 + (dn1 - dn0 - d1 - 3d0)x + (d0 - dn0) = 0.
	a := 2 * (d1 + d0)
	b := dn1 - dn0 - d1 - 3*d0
	cc := d0 - dn0

	var x float64
	if math.Abs(a) < 1e-12 {
		if math.Abs(b) < 1e-12 {
			return 0
		}
		x = -cc / b
	} else {
		disc := b*b - 4*a*cc
		if disc < 0 {
			return 0
		}
		sq := math.Sqrt(disc)
		x1 := (-b + sq) / (2 * a)
		x2 := (-b - sq) / (2 * a)
		x = x1
		if math.Abs(x2) < math.Abs(x1) {
			x = x2
		}
	}
	p := x / (x - 0.5)
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	return p
}

type stegoChan struct {
	ch       string
	chi      float64
	ent      float64
	rs       float64
	chiBits  [4]float64 // generalized chi-square at depths 1..4
	entPlane [4]float64 // entropy of planes 0..3
}

type stegoInspection struct {
	width, height, capacity int
	serconPresent           bool
	encrypted, text         bool
	payloadBytes            int
	headerBits              int // declared bit depth from the header (0 if none)
	estimatedBits           int // statistically estimated depth (0 if none)
	channels                []stegoChan
	suspicious              bool
	confidence              float64
	reasons                 []string
}

// Verdict thresholds (documented; tuned by tests).
const (
	stegoChiThreshold  = 0.5
	stegoEntThreshold  = 0.999
	stegoRSThreshold   = 0.10
	stegoBitsThreshold = 0.5 // mean generalized chi-square to credit a bit depth
)

// stegoInspect decodes carrier, runs the sercon magic check and the three
// per-channel signals, and computes the verdict.
func stegoInspect(carrier []byte) (stegoInspection, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return stegoInspection{}, err
	}
	rgba := toRGBA(img)
	b := rgba.Bounds()
	insp := stegoInspection{width: b.Dx(), height: b.Dy(), capacity: stegoCapacity(img, 1)}

	// sercon magic check (header is always embedded at 1 bit/channel).
	c := imageCarrier(rgba)
	if header, herr := lsbReadStream(c, stegoHeaderLen); herr == nil {
		if flags, length, perr := parseStegoHeader(header); perr == nil {
			insp.serconPresent = true
			insp.encrypted = flags&flagEncrypted != 0
			insp.text = flags&flagText != 0
			insp.payloadBytes = int(length)
			insp.headerBits = int((flags&flagBitsMask)>>flagBitsShift) + 1
		}
	}

	names := []string{"r", "g", "b"}
	var sumChi, sumEnt, sumRS float64
	var sumChiBits [4]float64
	chiHits := 0
	for c := 0; c < 3; c++ {
		vals := channelValues(rgba, c)
		chi := chiSquareLSB(vals)
		ent := lsbEntropy(vals)
		rs := rsAnalysis(rgba, c)
		var chiBits, entPlane [4]float64
		for d := 0; d < 4; d++ {
			chiBits[d] = chiSquareGroups(vals, d+1)
			entPlane[d] = entropyPlane(vals, d)
			sumChiBits[d] += chiBits[d]
		}
		insp.channels = append(insp.channels, stegoChan{ch: names[c], chi: chi, ent: ent, rs: rs, chiBits: chiBits, entPlane: entPlane})
		sumChi += chi
		sumEnt += ent
		sumRS += rs
		if chi >= stegoChiThreshold {
			chiHits++
		}
	}
	// estimatedBits: the largest depth whose mean generalized chi-square fires.
	for d := 0; d < 4; d++ {
		if sumChiBits[d]/3 >= stegoBitsThreshold {
			insp.estimatedBits = d + 1
		}
	}
	meanChi, meanEnt, meanRS := sumChi/3, sumEnt/3, sumRS/3

	var reasons []string
	if insp.serconPresent {
		reasons = append(reasons, "sercon stego header present")
	}
	if chiHits >= 2 {
		reasons = append(reasons, "chi-square indicates LSB embedding on multiple channels")
	}
	if meanEnt >= stegoEntThreshold {
		reasons = append(reasons, "LSB plane entropy near maximum")
	}
	if meanRS >= stegoRSThreshold {
		reasons = append(reasons, fmt.Sprintf("RS analysis estimates ~%.0f%% embedding", meanRS*100))
	}
	if insp.estimatedBits > 1 {
		reasons = append(reasons, fmt.Sprintf("statistics suggest ~%d-bit LSB embedding", insp.estimatedBits))
	}
	insp.reasons = reasons
	insp.suspicious = len(reasons) > 0

	switch {
	case insp.serconPresent:
		insp.confidence = 1.0
	case insp.suspicious:
		conf := meanChi
		if meanEnt >= stegoEntThreshold && meanEnt > conf {
			conf = meanEnt
		}
		if meanRS > conf {
			conf = meanRS
		}
		insp.confidence = conf
	default:
		insp.confidence = 0
	}
	return insp, nil
}

func (i stegoInspection) channelsJS() []any {
	out := make([]any, len(i.channels))
	for k, c := range i.channels {
		out[k] = map[string]any{
			"channel":         c.ch,
			"chiSquare":       c.chi,
			"lsbEntropy":      c.ent,
			"rsEstimate":      c.rs,
			"chiSquareByBits": []any{c.chiBits[0], c.chiBits[1], c.chiBits[2], c.chiBits[3]},
			"entropyByPlane":  []any{c.entPlane[0], c.entPlane[1], c.entPlane[2], c.entPlane[3]},
		}
	}
	return out
}

func reasonsJS(rs []string) []any {
	out := make([]any, len(rs))
	for k, r := range rs {
		out[k] = r
	}
	return out
}

// stegoAnalyze returns the full report map.
func stegoAnalyze(carrier []byte) (map[string]any, error) {
	insp, err := stegoInspect(carrier)
	if err != nil {
		return nil, err
	}
	sercon := map[string]any{"present": insp.serconPresent}
	if insp.serconPresent {
		sercon["encrypted"] = insp.encrypted
		sercon["text"] = insp.text
		sercon["payloadBytes"] = insp.payloadBytes
		sercon["bits"] = insp.headerBits
	}
	return map[string]any{
		"width":         insp.width,
		"height":        insp.height,
		"capacity":      insp.capacity,
		"estimatedBits": insp.estimatedBits,
		"sercon":        sercon,
		"channels":      insp.channelsJS(),
		"verdict": map[string]any{
			"suspicious": insp.suspicious,
			"confidence": insp.confidence,
			"reasons":    reasonsJS(insp.reasons),
		},
	}, nil
}

// stegoBitplane renders bit `plane` (0=LSB..7=MSB) of carrier as a PNG. For a
// single channel ("r"/"g"/"b") a set bit → white, unset → black (grayscale);
// for "rgb" each channel's bit maps to its colour component. Caller validates
// channel/plane.
func stegoBitplane(carrier []byte, channel string, plane int) ([]byte, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return nil, err
	}
	rgba := toRGBA(img)
	b := rgba.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	bit := byte(1) << uint(plane)
	set := func(on bool) byte {
		if on {
			return 255
		}
		return 0
	}
	n := len(rgba.Pix) / 4
	for p := 0; p < n; p++ {
		r := rgba.Pix[p*4]&bit != 0
		g := rgba.Pix[p*4+1]&bit != 0
		bl := rgba.Pix[p*4+2]&bit != 0
		var or, og, ob byte
		switch channel {
		case "r":
			v := set(r)
			or, og, ob = v, v, v
		case "g":
			v := set(g)
			or, og, ob = v, v, v
		case "b":
			v := set(bl)
			or, og, ob = v, v, v
		default: // "rgb"
			or, og, ob = set(r), set(g), set(bl)
		}
		out.Pix[p*4] = or
		out.Pix[p*4+1] = og
		out.Pix[p*4+2] = ob
		out.Pix[p*4+3] = 255
	}
	return encodeImage(out, "png", encodeOpts{})
}

// stegoDetect returns the quick-answer summary map.
func stegoDetect(carrier []byte) (map[string]any, error) {
	insp, err := stegoInspect(carrier)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"sercon":     insp.serconPresent,
		"suspicious": insp.suspicious,
		"confidence": insp.confidence,
	}
	if insp.serconPresent {
		m["encrypted"] = insp.encrypted
		m["text"] = insp.text
		m["payloadBytes"] = insp.payloadBytes
		m["bits"] = insp.headerBits
	}
	return m, nil
}


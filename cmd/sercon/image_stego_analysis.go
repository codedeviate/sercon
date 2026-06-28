// cmd/sercon/image_stego_analysis.go
package main

import (
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

// lsbEntropy is the Shannon entropy (bits) of the LSB bit-stream of vals:
// 0 when all LSBs are identical, 1.0 when perfectly balanced.
func lsbEntropy(vals []byte) float64 {
	if len(vals) == 0 {
		return 0
	}
	ones := 0
	for _, v := range vals {
		ones += int(v & 1)
	}
	p1 := float64(ones) / float64(len(vals))
	p0 := 1 - p1
	if p0 == 0 || p1 == 0 {
		return 0
	}
	return -(p0*math.Log2(p0) + p1*math.Log2(p1))
}

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

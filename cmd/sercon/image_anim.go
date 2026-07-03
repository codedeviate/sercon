// cmd/sercon/image_anim.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"

	"github.com/kettek/apng"
)

type animFrame struct {
	img              image.Image
	delayMs          int
	xOffset, yOffset int
	disposal         string // none | background | previous
	blend            string // source | over
}

type animDoc struct {
	format             string // gif | apng | <single-frame source format>
	width, height      int
	loopCount          int
	frames             []animFrame
}

func gifDisposalToStr(d byte) string {
	switch d {
	case gif.DisposalBackground:
		return "background"
	case gif.DisposalPrevious:
		return "previous"
	default: // 0 (unspecified) or gif.DisposalNone
		return "none"
	}
}

func apngDisposeToStr(d byte) string {
	switch d {
	case 1:
		return "background"
	case 2:
		return "previous"
	default:
		return "none"
	}
}

func apngBlendToStr(b byte) string {
	if b == 1 {
		return "over"
	}
	return "source"
}

// checkFramesPixelBudget rejects a container whose declared canvas
// dimensions exceed DefaultMaxImagePixels, mirroring decodeImage's guard for
// the static image path. image.DecodeConfig only parses the header (GIF's
// Logical Screen Descriptor / PNG's IHDR), so a crafted file can declare an
// extreme width x height while staying a few dozen bytes — gif.DecodeAll /
// apng.DecodeAll would otherwise allocate a full frame buffer sized from
// that declaration before any real pixel data is read ("decode bomb"). A
// DecodeConfig error is left for the real decoder to report, since that's a
// format problem unrelated to the pixel budget.
func checkFramesPixelBudget(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > DefaultMaxImagePixels {
		return fmt.Errorf("image.decodeFrames: image dimensions %dx%d exceed max pixels (%d)", cfg.Width, cfg.Height, DefaultMaxImagePixels)
	}
	return nil
}

func decodeFramesGIF(data []byte) (animDoc, error) {
	if err := checkFramesPixelBudget(data); err != nil {
		return animDoc{}, err
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return animDoc{}, fmt.Errorf("image.decodeFrames: gif: %w", err)
	}
	doc := animDoc{format: "gif", loopCount: g.LoopCount, width: g.Config.Width, height: g.Config.Height}
	for i, im := range g.Image {
		b := im.Bounds()
		disp := byte(0)
		if i < len(g.Disposal) {
			disp = g.Disposal[i]
		}
		delay := 0
		if i < len(g.Delay) {
			delay = g.Delay[i] * 10
		}
		doc.frames = append(doc.frames, animFrame{
			img: im, delayMs: delay, xOffset: b.Min.X, yOffset: b.Min.Y,
			disposal: gifDisposalToStr(disp), blend: "over",
		})
	}
	if doc.width == 0 || doc.height == 0 { // zero-valued Config → derive from frames
		doc.width, doc.height = frameExtent(doc.frames)
	}
	return doc, nil
}

// checkAPNGFramePixelBudget pre-scans the raw PNG/APNG chunk stream for fcTL
// chunks and rejects any whose declared frame width x height exceeds
// DefaultMaxImagePixels. This must run before apng.DecodeAll, not after:
// kettek/apng's decoder overwrites a frame's working width/height from its
// fcTL chunk (reader.go parsefcTL) and then allocates that frame's pixel
// buffer from those dimensions in readImagePass — before any row data is
// read — so a small IHDR canvas (which checkFramesPixelBudget validates via
// image.DecodeConfig) does not prevent an oversized fcTL from triggering the
// allocation. A malformed chunk stream is left for apng.DecodeAll itself to
// report, since that's a format problem unrelated to the pixel budget.
func checkAPNGFramePixelBudget(data []byte) error {
	const sigLen = 8
	if len(data) < sigLen {
		return nil
	}
	pos := sigLen
	for pos+8 <= len(data) {
		length := int64(binary.BigEndian.Uint32(data[pos : pos+4]))
		typ := string(data[pos+4 : pos+8])
		dataStart := pos + 8
		remaining := int64(len(data) - dataStart)
		if length < 0 || length > remaining {
			return nil // malformed chunk length; let apng.DecodeAll report it
		}
		if typ == "fcTL" && length >= 12 {
			w := int64(binary.BigEndian.Uint32(data[dataStart+4 : dataStart+8]))
			h := int64(binary.BigEndian.Uint32(data[dataStart+8 : dataStart+12]))
			if w*h > DefaultMaxImagePixels {
				return fmt.Errorf("image.decodeFrames: apng frame dimensions %dx%d exceed max pixels (%d)", w, h, DefaultMaxImagePixels)
			}
		}
		if typ == "IEND" {
			break
		}
		pos = dataStart + int(length) + 4 // skip chunk data + trailing CRC
	}
	return nil
}

func decodeFramesAPNG(data []byte) (animDoc, error) {
	if err := checkFramesPixelBudget(data); err != nil {
		return animDoc{}, err
	}
	if err := checkAPNGFramePixelBudget(data); err != nil {
		return animDoc{}, err
	}
	a, err := apng.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return animDoc{}, fmt.Errorf("image.decodeFrames: apng: %w", err)
	}
	doc := animDoc{format: "apng", loopCount: int(a.LoopCount)}
	for _, f := range a.Frames {
		if f.IsDefault {
			continue // default image is not part of the animation
		}
		den := int(f.DelayDenominator)
		if den == 0 {
			den = 100 // APNG: 0 denominator means 1/100s
		}
		doc.frames = append(doc.frames, animFrame{
			img: f.Image, delayMs: 1000 * int(f.DelayNumerator) / den,
			xOffset: f.XOffset, yOffset: f.YOffset,
			disposal: apngDisposeToStr(f.DisposeOp), blend: apngBlendToStr(f.BlendOp),
		})
	}
	// Static PNG: all frames are IsDefault — use the default frame as a single
	// animation frame so decodeFramesAny always returns at least one frame.
	if len(doc.frames) == 0 {
		for _, f := range a.Frames {
			if f.IsDefault && f.Image != nil {
				b := f.Image.Bounds()
				doc.format = "png"
				doc.frames = []animFrame{{img: f.Image, disposal: "none", blend: "over"}}
				doc.width, doc.height = b.Dx(), b.Dy()
				return doc, nil
			}
		}
	}
	doc.width, doc.height = frameExtent(doc.frames)
	return doc, nil
}

// frameExtent derives a canvas size from the frames' offsets + sizes.
func frameExtent(frames []animFrame) (int, int) {
	w, h := 0, 0
	for _, f := range frames {
		b := f.img.Bounds()
		if r := f.xOffset + b.Dx(); r > w {
			w = r
		}
		if bot := f.yOffset + b.Dy(); bot > h {
			h = bot
		}
	}
	return w, h
}

func strToGifDisposal(s string) byte {
	switch s {
	case "background":
		return gif.DisposalBackground
	case "previous":
		return gif.DisposalPrevious
	default:
		return gif.DisposalNone
	}
}

func strToApngDispose(s string) byte {
	switch s {
	case "background":
		return 1
	case "previous":
		return 2
	default:
		return 0
	}
}

func strToApngBlend(s string) byte {
	if s == "over" {
		return 1
	}
	return 0
}

// palettize converts img to a paletted frame at the given bounds (offset+size)
// using the Plan9 256-color palette with Floyd–Steinberg dithering.
func palettize(img image.Image, bounds image.Rectangle) *image.Paletted {
	p := image.NewPaletted(bounds, palette.Plan9)
	draw.FloydSteinberg.Draw(p, bounds, img, img.Bounds().Min)
	return p
}

func encodeFramesGIF(doc animDoc) ([]byte, error) {
	if len(doc.frames) == 0 {
		return nil, fmt.Errorf("image.encodeFrames: gif requires at least one frame")
	}
	w, h := doc.width, doc.height
	if w == 0 || h == 0 {
		w, h = frameExtent(doc.frames)
	}
	g := &gif.GIF{LoopCount: doc.loopCount, Config: image.Config{ColorModel: color.Palette(palette.Plan9), Width: w, Height: h}}
	for _, f := range doc.frames {
		fb := f.img.Bounds()
		bounds := image.Rect(f.xOffset, f.yOffset, f.xOffset+fb.Dx(), f.yOffset+fb.Dy())
		g.Image = append(g.Image, palettize(f.img, bounds))
		g.Delay = append(g.Delay, f.delayMs/10)
		g.Disposal = append(g.Disposal, strToGifDisposal(f.disposal))
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		return nil, fmt.Errorf("image.encodeFrames: %w", err)
	}
	return b.Bytes(), nil
}

func encodeFramesAPNG(doc animDoc) ([]byte, error) {
	if len(doc.frames) == 0 {
		return nil, fmt.Errorf("image.encodeFrames: apng requires at least one frame")
	}
	a := apng.APNG{LoopCount: uint(doc.loopCount)}
	for _, f := range doc.frames {
		num, den := f.delayMs, 1000
		if num > 65535 { // exact-ms denominator would overflow uint16; rescale to centiseconds
			num, den = (f.delayMs+5)/10, 100
			if num > 65535 {
				num = 65535 // cap ~655s
			}
		}
		a.Frames = append(a.Frames, apng.Frame{
			Image:            f.img,
			XOffset:          f.xOffset,
			YOffset:          f.yOffset,
			DelayNumerator:   uint16(num),
			DelayDenominator: uint16(den),
			DisposeOp:        strToApngDispose(f.disposal),
			BlendOp:          strToApngBlend(f.blend),
		})
	}
	var b bytes.Buffer
	if err := apng.Encode(&b, a); err != nil {
		return nil, fmt.Errorf("image.encodeFrames: %w", err)
	}
	return b.Bytes(), nil
}

// decodeFramesAny sniffs the container and dispatches. GIF → all frames; PNG →
// APNG decoder (handles static + animated); anything else → a single frame.
func decodeFramesAny(data []byte) (animDoc, error) {
	_, format, err := decodeImage(data)
	if err != nil {
		return animDoc{}, fmt.Errorf("image.decodeFrames: %w", err)
	}
	switch format {
	case "gif":
		return decodeFramesGIF(data)
	case "png":
		return decodeFramesAPNG(data)
	default:
		img, _, derr := decodeImage(data)
		if derr != nil {
			return animDoc{}, fmt.Errorf("image.decodeFrames: %w", derr)
		}
		b := img.Bounds()
		return animDoc{
			format: format, width: b.Dx(), height: b.Dy(), loopCount: 0,
			frames: []animFrame{{img: img, disposal: "none", blend: "over"}},
		}, nil
	}
}

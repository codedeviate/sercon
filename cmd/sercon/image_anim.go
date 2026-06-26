// cmd/sercon/image_anim.go
package main

import (
	"bytes"
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

func decodeFramesGIF(data []byte) (animDoc, error) {
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

func decodeFramesAPNG(data []byte) (animDoc, error) {
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

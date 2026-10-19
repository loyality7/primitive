package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loyality7/primitive/primitive/imageio"
	"github.com/loyality7/primitive/primitive/quad"
)

// flat returns a w x h image filled with c, with an optional square of d in the corner.
func flat(w, h int, c color.RGBA, sq int, d color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(im, im.Rect, &image.Uniform{c}, image.Point{}, draw.Src)
	if sq > 0 {
		draw.Draw(im, image.Rect(0, 0, sq, sq), &image.Uniform{d}, image.Point{}, draw.Src)
	}
	return im
}

// compose plays the frames of a GIF onto a canvas, honoring offsets and transparency.
func compose(t *testing.T, g *gif.GIF) []*image.RGBA {
	t.Helper()
	w, h := g.Config.Width, g.Config.Height
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	var out []*image.RGBA
	for _, f := range g.Image {
		draw.Draw(canvas, f.Bounds(), f, f.Bounds().Min, draw.Over)
		snap := image.NewRGBA(canvas.Rect)
		copy(snap.Pix, canvas.Pix)
		out = append(out, snap)
	}
	return out
}

func decode(t *testing.T, data []byte) *gif.GIF {
	t.Helper()
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the GIF does not decode: %v", err)
	}
	return g
}

var (
	red   = color.RGBA{R: 220, G: 30, B: 30, A: 255}
	blue  = color.RGBA{R: 20, G: 40, B: 230, A: 255}
	green = color.RGBA{R: 30, G: 200, B: 60, A: 255}
)

func TestGIFKeepsFlatColorsExactly(t *testing.T) {
	frames := []image.Image{flat(40, 30, red, 0, red), flat(40, 30, red, 10, blue), flat(40, 30, green, 20, blue)}
	data, err := imageio.EncodeGIF(frames, 7, 250)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:6]) != "GIF89a" {
		t.Fatalf("bad header %q", data[:6])
	}
	g := decode(t, data)
	if g.Config.Width != 40 || g.Config.Height != 30 || len(g.Image) != 3 {
		t.Fatalf("size %dx%d with %d frames", g.Config.Width, g.Config.Height, len(g.Image))
	}
	if g.LoopCount != 0 {
		t.Fatalf("LoopCount = %d, want 0 (loop forever)", g.LoopCount)
	}
	if g.Delay[0] != 7 || g.Delay[2] != 250 {
		t.Fatalf("delays %v, want 7 first and 250 last", g.Delay)
	}
	for i, got := range compose(t, g) {
		want := frames[i].(*image.RGBA)
		for p := 0; p < len(want.Pix); p++ {
			if got.Pix[p] != want.Pix[p] {
				t.Fatalf("frame %d byte %d: got %d want %d (flat colors must survive exactly)", i, p, got.Pix[p], want.Pix[p])
			}
		}
	}
}

func TestGIFStoresOnlyWhatChanged(t *testing.T) {
	frames := []image.Image{flat(60, 60, red, 0, red), flat(60, 60, red, 8, blue)}
	g := decode(t, mustEncode(t, frames, 5, 5))
	if r := g.Image[1].Bounds(); r.Dx() != 8 || r.Dy() != 8 {
		t.Fatalf("second frame covers %v, want only the 8x8 square that changed", r)
	}
}

func TestGIFMergesIdenticalFrames(t *testing.T) {
	a := flat(20, 20, red, 0, red)
	g := decode(t, mustEncode(t, []image.Image{a, a, a}, 4, 100))
	if len(g.Image) != 1 || g.Delay[0] != 4+4+100 {
		t.Fatalf("got %d frames with delays %v, want 1 frame shown for 108", len(g.Image), g.Delay)
	}
}

func TestGIFRejectsBadInput(t *testing.T) {
	if _, err := imageio.EncodeGIF(nil, 5, 5); err == nil {
		t.Error("no frames should be an error")
	}
	if _, err := imageio.EncodeGIF([]image.Image{flat(10, 10, red, 0, red), flat(11, 10, red, 0, red)}, 5, 5); err == nil {
		t.Error("frames of different sizes should be an error")
	}
}

func mustEncode(t *testing.T, frames []image.Image, delay, last int) []byte {
	t.Helper()
	data, err := imageio.EncodeGIF(frames, delay, last)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// meanError is the average absolute difference per color channel.
func meanError(a, b *image.RGBA) float64 {
	sum := 0
	for i := 0; i < len(a.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := int(a.Pix[i+c]) - int(b.Pix[i+c])
			if d < 0 {
				d = -d
			}
			sum += d
		}
	}
	return float64(sum) / float64(len(a.Pix)/4*3)
}

func TestGIFOfARealPictureIsCloseToTheOriginal(t *testing.T) {
	// a smooth gradient has far more than 256 colors, so this tests the palette
	m := quad.NewModel(gradientImage(), 96)
	m.Border = 0
	for i := 0; i < 400; i++ {
		m.Split()
	}
	frames := m.Frames(0)
	g := decode(t, mustEncode(t, frames, 6, 100))
	if len(g.Image) < 10 {
		t.Fatalf("only %d frames", len(g.Image))
	}
	played := compose(t, g)
	last, want := played[len(played)-1], m.Image().(*image.RGBA)
	if e := meanError(last, want); e > 6 {
		t.Fatalf("the last frame is off by %.1f levels per channel on average, want under 6", e)
	}
	// every frame, not just the last, must be close to the frame that went in
	for i := range played {
		if e := meanError(played[i], frames[i].(*image.RGBA)); e > 8 {
			t.Fatalf("frame %d is off by %.1f", i, e)
		}
	}
}

func TestCLIWritesGIFWithoutImageMagick(t *testing.T) {
	bin, dir := buildCLI(t)
	for name, args := range map[string][]string{
		"quads":  {"-algo", "quads", "-n", "120"},
		"shapes": {"-m", "1", "-n", "8"},
	} {
		out := filepath.Join(dir, name+".gif")
		full := append([]string{"-i", "../examples/owl.png", "-o", out, "-r", "96"}, args...)
		if b, err := exec.Command(bin, full...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, b)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if g := decode(t, data); len(g.Image) < 3 {
			t.Fatalf("%s: only %d frames", name, len(g.Image))
		}
	}
}

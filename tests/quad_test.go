package tests

import (
	"image"
	"image/color"
	"math/rand"
	"strings"
	"testing"

	"github.com/loyality7/primitive/primitive/quad"
)

func noisyImage(w, h int, seed int64) *image.RGBA {
	rnd := rand.New(rand.NewSource(seed))
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetRGBA(x, y, color.RGBA{R: uint8(rnd.Intn(256)), G: uint8(rnd.Intn(256)), B: uint8(rnd.Intn(256)), A: 255})
		}
	}
	return im
}

func TestQuadErrorShrinks(t *testing.T) {
	m := quad.NewModel(gradientImage(), 48)
	prev := m.Score()
	if prev <= 0 {
		t.Fatal("one flat rectangle must have an error on a gradient")
	}
	for i := 0; i < 200; i++ {
		if !m.Split() {
			t.Fatalf("ran out of splits after %d", i)
		}
		if s := m.Score(); s > prev+1e-9 {
			t.Fatalf("score went up at split %d: %v -> %v", i, prev, s)
		}
		prev = m.Score()
	}
	if m.Score() > 0.02 {
		t.Fatalf("200 splits should fit a smooth gradient well, got %v", m.Score())
	}
}

func TestQuadLeafCount(t *testing.T) {
	m := quad.NewModel(noisyImage(64, 64, 1), 64)
	for i := 0; i < 50; i++ {
		m.Split()
	}
	if got := m.Leaves(); got != 1+3*50 {
		t.Fatalf("leaves = %d, want %d", got, 1+3*50)
	}
}

func TestQuadExactWhenFullySplit(t *testing.T) {
	src := noisyImage(8, 8, 2)
	m := quad.NewModel(src, 8)
	for m.Split() {
	}
	if m.Score() != 0 {
		t.Fatalf("fully split score = %v, want 0", m.Score())
	}
	out := m.Image().(*image.RGBA)
	for i := range src.Pix {
		if out.Pix[i] != src.Pix[i] {
			t.Fatalf("pixel byte %d differs: got %d want %d", i, out.Pix[i], src.Pix[i])
		}
	}
}

func TestQuadTilesNonSquareImage(t *testing.T) {
	m := quad.NewModel(noisyImage(40, 24, 3), 100)
	for i := 0; i < 30; i++ {
		m.Split()
	}
	out := m.Image().(*image.RGBA)
	if b := out.Bounds(); b.Dx() != 100 || b.Dy() != 60 {
		t.Fatalf("output size %v, want 100x60", b)
	}
	for i := 3; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 255 {
			t.Fatalf("gap in the output at byte %d", i)
		}
	}
}

func TestQuadFlatImageStops(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range im.Pix {
		im.Pix[i] = 200
	}
	m := quad.NewModel(im, 32)
	if m.Split() {
		t.Fatal("a flat image has nothing to split")
	}
	if m.Leaves() != 1 || m.Score() != 0 {
		t.Fatalf("leaves=%d score=%v", m.Leaves(), m.Score())
	}
}

func TestQuadSVG(t *testing.T) {
	m := quad.NewModel(noisyImage(16, 16, 4), 16)
	for i := 0; i < 5; i++ {
		m.Split()
	}
	svg := m.SVG()
	if got := strings.Count(svg, "<rect"); got != m.Leaves() {
		t.Fatalf("%d rects for %d leaves", got, m.Leaves())
	}
	if !strings.Contains(svg, `viewBox="0 0 16 16"`) {
		t.Fatal("viewBox missing")
	}
}

func TestQuadFrames(t *testing.T) {
	m := quad.NewModel(noisyImage(32, 32, 5), 32)
	for i := 0; i < 100; i++ {
		m.Split()
	}
	frames := m.Frames(0)
	if len(frames) < 2 || len(frames) > 60 {
		t.Fatalf("%d frames, want between 2 and 60", len(frames))
	}
	first, last := frames[0].(*image.RGBA), frames[len(frames)-1].(*image.RGBA)
	for i := 4; i < len(first.Pix); i += 4 {
		if first.Pix[i] != first.Pix[0] || first.Pix[i+1] != first.Pix[1] {
			t.Fatal("first frame should be one flat color")
		}
	}
	final := m.Image().(*image.RGBA)
	for i := range final.Pix {
		if last.Pix[i] != final.Pix[i] {
			t.Fatal("last frame must equal the current image")
		}
	}
}

package tests

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"

	"github.com/loyality7/primitive/primitive/quad"
)

func solid(w, h int, v uint8) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(im, im.Rect, &image.Uniform{color.RGBA{R: v, G: v, B: v, A: 255}}, image.Point{}, draw.Src)
	return im
}

func samePixels(t *testing.T, a, b *image.RGBA, what string) {
	t.Helper()
	if a.Rect != b.Rect {
		t.Fatalf("%s: sizes differ %v %v", what, a.Rect, b.Rect)
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("%s: pixel byte %d differs (%d vs %d)", what, i, a.Pix[i], b.Pix[i])
		}
	}
}

func TestLiveModelMatchesNewModel(t *testing.T) {
	src := noisyImage(40, 30, 21)
	a := quad.NewModel(src, 80)
	b := quad.NewEmpty(40, 30, 80)
	if err := b.SetFrame(src, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		a.Split()
		b.Split()
	}
	if a.Score() != b.Score() || a.SVG() != b.SVG() {
		t.Fatal("a model filled with SetFrame must equal one made by NewModel")
	}
	samePixels(t, a.Image().(*image.RGBA), b.Image().(*image.RGBA), "image")
}

func TestSetFrameRestartsTheTree(t *testing.T) {
	m := quad.NewEmpty(32, 32, 32)
	m.SetFrame(noisyImage(32, 32, 1), 0)
	for i := 0; i < 50; i++ {
		m.Split()
	}
	other := noisyImage(32, 32, 2)
	m.SetFrame(other, 0)
	if m.Leaves() != 1 {
		t.Fatalf("%d leaves after a new frame, want 1", m.Leaves())
	}
	if want := quad.NewModel(other, 32).Score(); m.Score() != want {
		t.Fatalf("score %v, want %v for the new frame", m.Score(), want)
	}
	for i := 0; i < 20; i++ {
		m.Split()
	}
	if m.Leaves() != 1+3*20 {
		t.Fatalf("%d leaves after 20 splits", m.Leaves())
	}
}

func TestSetFrameRejectsWrongSize(t *testing.T) {
	m := quad.NewEmpty(32, 32, 32)
	if err := m.SetFrame(noisyImage(31, 32, 1), 0); err == nil {
		t.Fatal("a frame of another size should be an error")
	}
}

func TestSmoothingBlendsFrames(t *testing.T) {
	m := quad.NewEmpty(16, 16, 16)
	m.Border = 0
	out := image.NewRGBA(image.Rect(0, 0, 16, 16))
	grey := func() uint8 { m.RenderInto(out); return out.Pix[0] }

	m.SetFrame(solid(16, 16, 0), 0.5)
	if g := grey(); g != 0 {
		t.Fatalf("first frame should start the average at black, got %d", g)
	}
	m.SetFrame(solid(16, 16, 255), 0.5)
	if g := grey(); g < 126 || g > 129 {
		t.Fatalf("half way between black and white should be about 128, got %d", g)
	}
	m.SetFrame(solid(16, 16, 255), 0.5)
	if g := grey(); g < 189 || g > 193 {
		t.Fatalf("three quarters of the way should be about 191, got %d", g)
	}
	m.SetFrame(solid(16, 16, 255), 0) // no smoothing: the frame as it is
	if g := grey(); g != 255 {
		t.Fatalf("smoothing off should give the frame itself, got %d", g)
	}
	m.SetFrame(solid(16, 16, 0), 0.5) // turning it on again starts fresh, not from the old white
	if g := grey(); g != 0 {
		t.Fatalf("smoothing restarted should begin at the new frame, got %d", g)
	}
}

func TestNoHistoryGivesTheSamePicture(t *testing.T) {
	src := noisyImage(48, 48, 5)
	a := quad.NewModel(src, 96)
	b := quad.NewEmpty(48, 48, 96)
	b.NoHistory = true
	b.SetFrame(src, 0)
	for i := 0; i < 200; i++ {
		a.Split()
		b.Split()
	}
	if a.Score() != b.Score() || a.Leaves() != b.Leaves() {
		t.Fatalf("without history: score %v leaves %d, with: %v %d", b.Score(), b.Leaves(), a.Score(), a.Leaves())
	}
	samePixels(t, a.Image().(*image.RGBA), b.Image().(*image.RGBA), "image")
}

func TestRenderIntoReusesAnyImage(t *testing.T) {
	for _, circles := range []bool{false, true} {
		m := quad.NewModel(noisyImage(36, 24, 8), 72)
		m.Circles = circles
		for i := 0; i < 60; i++ {
			m.Split()
		}
		dirty := image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
		for i := range dirty.Pix {
			dirty.Pix[i] = 77 // leftovers of a previous frame
		}
		m.RenderInto(dirty)
		samePixels(t, dirty, m.Image().(*image.RGBA), "RenderInto over a dirty image")
	}
}

func TestLiveFramesAreFastEnough(t *testing.T) {
	m := quad.NewEmpty(320, 240, 512)
	m.NoHistory = true
	out := image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
	frames := []*image.RGBA{noisyImage(320, 240, 1), noisyImage(320, 240, 2)}
	start := time.Now()
	const n = 20
	for i := 0; i < n; i++ {
		m.SetFrame(frames[i%2], 0.5)
		for s := 0; s < 1500; s++ {
			m.Split()
		}
		m.RenderInto(out)
	}
	per := time.Since(start) / n
	t.Logf("one 320x240 frame with 1500 splits and a 512 px render: %v", per)
	// worst case input (pure noise) and a slow CI machine still stay far from a second
	if per > 250*time.Millisecond {
		t.Fatalf("%v per frame is too slow for live video", per)
	}
}

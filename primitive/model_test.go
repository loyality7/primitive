package primitive

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func gradientImage() image.Image {
	im := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x * 5), uint8(y * 5), 120, 255})
		}
	}
	return im
}

func run(seed int64, steps int) *Model {
	m := NewModel(gradientImage(), Color{128, 128, 128, 255}, 48, 1, seed)
	for i := 0; i < steps; i++ {
		m.Step(ShapeTypeTriangle, 128, 0)
	}
	return m
}

func TestSnapToPalette(t *testing.T) {
	defer func() { Palette = nil }()
	Palette = []Color{{0, 0, 0, 255}, {255, 0, 0, 255}, {255, 255, 255, 255}}
	cases := []struct{ in, want Color }{
		{Color{10, 10, 10, 77}, Color{0, 0, 0, 77}},      // alpha is kept
		{Color{240, 30, 20, 128}, Color{255, 0, 0, 128}}, // nearest is red
		{Color{200, 200, 200, 1}, Color{255, 255, 255, 1}},
	}
	for _, c := range cases {
		if got := snapToPalette(c.in); got != c.want {
			t.Errorf("snapToPalette(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	Palette = nil
	if in := (Color{1, 2, 3, 4}); snapToPalette(in) != in {
		t.Error("empty palette must leave colors untouched")
	}
}

func TestSameSeedSameResult(t *testing.T) {
	a, b := run(5, 8), run(5, 8)
	if a.Score != b.Score || a.SVG() != b.SVG() {
		t.Fatal("same seed with one worker must give identical output")
	}
	if c := run(6, 8); c.SVG() == a.SVG() {
		t.Fatal("different seeds should give different output")
	}
}

func TestPaletteLimitsModelColors(t *testing.T) {
	defer func() { Palette = nil }()
	Palette = []Color{{0, 0, 0, 255}, {255, 255, 255, 255}}
	m := run(3, 10)
	for i, c := range m.Colors {
		if (c.R != 0 || c.G != 0 || c.B != 0) && (c.R != 255 || c.G != 255 || c.B != 255) {
			t.Fatalf("shape %d color %v is not in the palette", i, c)
		}
	}
}

func TestAnimatedSVG(t *testing.T) {
	m := run(5, 3)
	if s := m.SVG(); strings.Contains(s, "class=\"a\"") || strings.Contains(s, "<style>") {
		t.Fatal("static SVG must not contain animation markup")
	}
	m.AnimStep = 0.1
	s := m.SVG()
	if !strings.Contains(s, "animation-delay:0.200s") || !strings.Contains(s, "<style>") {
		t.Fatal("animated SVG missing style or per-shape delay")
	}
}

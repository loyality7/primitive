package tests

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/loyality7/primitive/primitive"
	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/shape"
)

func gradientImage() image.Image {
	im := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			im.SetRGBA(x, y, color.RGBA{R: uint8(x * 5), G: uint8(y * 5), B: 120, A: 255})
		}
	}
	return im
}

func newModel(seed int64) *primitive.Model {
	return primitive.NewModel(gradientImage(), colors.Color{R: 128, G: 128, B: 128, A: 255}, 48, 1, seed)
}

func run(seed int64, steps int) *primitive.Model {
	m := newModel(seed)
	for i := 0; i < steps; i++ {
		m.Step(shape.TypeTriangle, 128, 0)
	}
	return m
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

func TestEveryShapeTypeRuns(t *testing.T) {
	types := []shape.Type{
		shape.TypeTriangle, shape.TypeRectangle, shape.TypeEllipse, shape.TypeCircle,
		shape.TypeRotatedRectangle, shape.TypeQuadratic, shape.TypeRotatedEllipse, shape.TypePolygon, shape.TypeAny,
	}
	for _, typ := range types {
		m := newModel(1)
		for i := 0; i < 3; i++ {
			m.Step(typ, 128, 0)
		}
		if len(m.Shapes) != 3 {
			t.Errorf("shape type %d: got %d shapes, want 3", typ, len(m.Shapes))
		}
	}
}

func TestPaletteLimitsModelColors(t *testing.T) {
	m := newModel(3)
	m.SetPalette([]colors.Color{{R: 0, G: 0, B: 0, A: 255}, {R: 255, G: 255, B: 255, A: 255}})
	for i := 0; i < 10; i++ {
		m.Step(shape.TypeTriangle, 128, 0)
	}
	for i, c := range m.Colors {
		black := c.R == 0 && c.G == 0 && c.B == 0
		white := c.R == 255 && c.G == 255 && c.B == 255
		if !black && !white {
			t.Fatalf("shape %d color %v is not in the palette", i, c)
		}
	}
}

func TestPaletteIsPerModel(t *testing.T) {
	a := newModel(3)
	a.SetPalette([]colors.Color{{R: 255, G: 0, B: 0, A: 255}})
	b := newModel(3) // must not inherit the palette of a
	b.Step(shape.TypeTriangle, 128, 0)
	if c := b.Colors[0]; c.R == 255 && c.G == 0 && c.B == 0 {
		t.Fatal("palette leaked between models")
	}
}

func TestAnimatedSVG(t *testing.T) {
	m := run(5, 3)
	if s := m.SVG(); strings.Contains(s, `class="a"`) || strings.Contains(s, "<style>") {
		t.Fatal("static SVG must not contain animation markup")
	}
	m.AnimStep = 0.1
	s := m.SVG()
	if !strings.Contains(s, "animation-delay:0.200s") || !strings.Contains(s, "<style>") {
		t.Fatal("animated SVG missing style or per-shape delay")
	}
}

func TestBlurSVG(t *testing.T) {
	m := run(5, 3)
	if strings.Contains(m.SVG(), "feGaussianBlur") {
		t.Fatal("blur must be off by default")
	}
	m.Blur = 2.5
	s := m.SVG()
	if !strings.Contains(s, `stdDeviation="2.5"`) || !strings.Contains(s, `filter="url(#b)"`) {
		t.Fatal("blurred SVG missing filter definition or reference")
	}
}

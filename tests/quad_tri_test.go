package tests

import (
	"image"
	"image/color"
	"regexp"
	"strings"
	"testing"

	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/quad"
)

var (
	triRed  = color.RGBA{R: 230, G: 40, B: 40, A: 255}
	triBlue = color.RGBA{R: 30, G: 60, B: 220, A: 255}
)

// twoTriangles is a w x h picture cut by a diagonal, with the same rule the
// engine uses: a pixel belongs to the side its center is on.
func twoTriangles(w, h, diag int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			first := (2*x+1)*h > (2*y+1)*w // diagonal 0: upper right
			if diag == 1 {
				first = (2*x+1)*h+(2*y+1)*w < 2*w*h // diagonal 1: upper left
			}
			c := triBlue
			if first {
				c = triRed
			}
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

func triModel(src *image.RGBA, size int) *quad.Model {
	m := quad.NewModel(src, size)
	m.Triangles = true
	m.Border = 0
	m.Restart()
	return m
}

func TestTrianglesFitTwoTrianglesExactly(t *testing.T) {
	for _, c := range []struct{ w, h, diag int }{{32, 32, 0}, {32, 32, 1}, {40, 24, 0}, {40, 24, 1}, {17, 29, 0}, {17, 29, 1}} {
		src := twoTriangles(c.w, c.h, c.diag)
		if s := triModel(src, c.w).Score(); s != 0 {
			t.Errorf("%dx%d diagonal %d: triangles should fit with no error, got %v", c.w, c.h, c.diag, s)
		}
		flat := quad.NewModel(src, c.w)
		if s := flat.Score(); s < 0.1 {
			t.Errorf("%dx%d diagonal %d: one flat color cannot fit two triangles, got %v", c.w, c.h, c.diag, s)
		}
	}
}

func TestTriangleRenderingKeepsTheColors(t *testing.T) {
	src := twoTriangles(32, 32, 0)
	m := triModel(src, 32)
	out := m.Image().(*image.RGBA)
	same := func(x, y int, c color.RGBA) bool {
		o := out.PixOffset(x, y)
		return out.Pix[o] == c.R && out.Pix[o+1] == c.G && out.Pix[o+2] == c.B
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if d := x - y; d > 1 || d < -1 { // away from the anti-aliased diagonal
				want := triBlue
				if d > 0 {
					want = triRed
				}
				if !same(x, y, want) {
					t.Fatalf("pixel %d,%d should be the exact triangle color", x, y)
				}
			}
		}
	}
	if same(16, 16, triRed) || same(16, 16, triBlue) {
		t.Fatal("a pixel on the diagonal should blend the two colors")
	}
}

func TestFullySplitTrianglesAreExact(t *testing.T) {
	src := noisyImage(24, 24, 3)
	m := triModel(src, 24)
	for m.Split() {
	}
	if m.Score() != 0 {
		t.Fatalf("score %v after splitting everything", m.Score())
	}
	out := m.Image().(*image.RGBA)
	for i := range src.Pix {
		if out.Pix[i] != src.Pix[i] {
			t.Fatalf("pixel byte %d differs", i)
		}
	}
}

func TestTrianglesBeatSquaresOnDiagonalEdges(t *testing.T) {
	src := twoTriangles(64, 64, 1)
	flat := quad.NewModel(src, 64)
	tri := triModel(src, 64)
	for i := 0; i < 12; i++ {
		flat.Split()
		tri.Split()
	}
	if tri.Score() >= flat.Score() {
		t.Fatalf("triangles %v should fit a diagonal edge better than squares %v", tri.Score(), flat.Score())
	}
}

func TestTriangleSVG(t *testing.T) {
	m := triModel(noisyImage(64, 64, 4), 64)
	m.Border = 1
	for i := 0; i < 20; i++ {
		m.Split()
	}
	svg := m.SVG()
	if got := strings.Count(svg, "<polygon"); got != 2*m.Leaves() {
		t.Fatalf("%d polygons for %d cells, want two each", got, m.Leaves())
	}
	if strings.Contains(svg, "<rect") || !strings.Contains(svg, "rect,polygon{stroke:#000") {
		t.Fatal("triangle cells use polygons and the outline style must cover them")
	}
	m.AnimStep = 0.1
	if got := strings.Count(m.SVG(), "<polygon"); got != 2*(1+4*20) {
		t.Fatalf("animated SVG has %d polygons, want %d", got, 2*(1+4*20))
	}
}

func TestPaletteSnapsCells(t *testing.T) {
	pal := []colors.Color{{R: 0, G: 0, B: 0, A: 255}, {R: 255, G: 255, B: 255, A: 255}}
	m := quad.NewModel(gradientImage(), 48)
	m.Border = 0
	m.Palette = pal
	for i := 0; i < 80; i++ {
		m.Split()
	}
	out := m.Image().(*image.RGBA)
	for i := 0; i < len(out.Pix); i += 4 {
		r, g, b := out.Pix[i], out.Pix[i+1], out.Pix[i+2]
		if !(r == 0 && g == 0 && b == 0) && !(r == 255 && g == 255 && b == 255) {
			t.Fatalf("pixel %d is %d,%d,%d, not in the palette", i/4, r, g, b)
		}
	}
	// the same palette in the SVG, for squares and for triangles
	for _, tris := range []bool{false, true} {
		m.Triangles = tris
		m.Restart()
		for i := 0; i < 30; i++ {
			m.Split()
		}
		for _, hex := range regexp.MustCompile(`fill="#([0-9a-f]{6})"`).FindAllStringSubmatch(m.SVG(), -1) {
			if hex[1] != "000000" && hex[1] != "ffffff" {
				t.Fatalf("triangles=%v: fill #%s is not in the palette", tris, hex[1])
			}
		}
	}
}

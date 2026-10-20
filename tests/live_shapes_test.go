package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/loyality7/primitive/primitive"
	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/shape"
)

func tinted(w, h int, c color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// a soft gradient so shapes have something to fit
			v := uint8(x * 120 / w)
			im.SetRGBA(x, y, color.RGBA{R: c.R/2 + v, G: c.G/2 + v/2, B: c.B / 2, A: 255})
		}
	}
	return im
}

func liveModel(src *image.RGBA, seed int64) *primitive.Model {
	return primitive.NewModel(src, colors.Color{R: 128, G: 128, B: 128, A: 255}, 48, 1, seed)
}

func grow(m *primitive.Model, n int) {
	for i := 0; i < n; i++ {
		m.Step(shape.TypeTriangle, 128, 0)
	}
}

func TestRecolorWithTheSameFrameChangesNothing(t *testing.T) {
	src := tinted(48, 48, color.RGBA{R: 200, G: 60, B: 40})
	m := liveModel(src, 1)
	grow(m, 25)
	before, svg := m.Score, m.SVG()
	if err := m.SetTarget(src, 0); err != nil {
		t.Fatal(err)
	}
	if d := m.Score - before; d > 1e-9 || d < -1e-9 {
		t.Fatalf("score %v became %v after recoloring for the same picture", before, m.Score)
	}
	if m.SVG() != svg {
		t.Fatal("the colors changed although the picture did not")
	}
}

func TestRecolorFollowsANewFrame(t *testing.T) {
	red := tinted(48, 48, color.RGBA{R: 240, G: 20, B: 20})
	blue := tinted(48, 48, color.RGBA{R: 20, G: 20, B: 240})
	m := liveModel(red, 2)
	grow(m, 30)
	fitted := m.Score
	if err := m.SetTarget(blue, 0); err != nil {
		t.Fatal(err)
	}
	// the shapes stay where they are and take the new colors, with no new search
	if m.Score > 3*fitted+0.05 {
		t.Fatalf("after the picture turned blue the score is %v, it was %v before: the shapes did not follow", m.Score, fitted)
	}
	if len(m.Shapes) != 30 {
		t.Fatalf("%d shapes, recoloring must keep them all", len(m.Shapes))
	}
	if err := m.SetTarget(tinted(40, 48, color.RGBA{}), 0); err == nil {
		t.Fatal("a frame of another size should be an error")
	}
}

func TestPruneDropsTheShapesThatHelpLeast(t *testing.T) {
	src := tinted(48, 48, color.RGBA{R: 90, G: 200, B: 70})
	m := liveModel(src, 3)
	grow(m, 12)
	good := m.Score
	// ten copies of the last shape on top of itself: they add nothing
	typ, params := shape.Encode(m.Shapes[len(m.Shapes)-1])
	for i := 0; i < 10; i++ {
		sh, err := shape.Decode(m.Workers[0].Env, typ, params)
		if err != nil {
			t.Fatal(err)
		}
		m.Add(sh, 128)
	}
	m.Recolor()
	m.Prune(12)
	if len(m.Shapes) != 12 {
		t.Fatalf("%d shapes after Prune(12)", len(m.Shapes))
	}
	if m.Score > good*1.05+1e-6 {
		t.Fatalf("pruning the useless copies made the picture worse: %v -> %v", good, m.Score)
	}
	m.Prune(100) // asking to keep more than there are changes nothing
	if len(m.Shapes) != 12 {
		t.Fatal("Prune must not add shapes")
	}
}

// Several copies of the live picture run the same steps. They must stay the
// same, or the threads of the browser would drift apart.
func TestLiveCopiesStayIdentical(t *testing.T) {
	frames := []*image.RGBA{
		tinted(48, 48, color.RGBA{R: 220, G: 40, B: 40}),
		tinted(48, 48, color.RGBA{R: 200, G: 90, B: 40}),
		tinted(48, 48, color.RGBA{R: 120, G: 160, B: 60}),
		tinted(48, 48, color.RGBA{R: 40, G: 150, B: 200}),
	}
	models := []*primitive.Model{liveModel(frames[0], 11), liveModel(frames[0], 22), liveModel(frames[0], 33)}
	const cap = 8
	for round := 0; round < 14; round++ {
		frame := frames[round%len(frames)]
		var best *primitive.State
		for _, m := range models {
			if err := m.SetTarget(frame, 0.3); err != nil {
				t.Fatal(err)
			}
		}
		for _, m := range models {
			if s := m.Search(shape.TypeAny, 128, 2); best == nil || s.Energy() < best.Energy() {
				best = s
			}
		}
		typ, params := shape.Encode(best.Shape)
		for _, m := range models {
			sh, err := shape.Decode(m.Workers[0].Env, typ, params)
			if err != nil {
				t.Fatal(err)
			}
			m.Add(sh, best.Alpha)
			m.Prune(cap)
		}
		for i, m := range models[1:] {
			if m.Score != models[0].Score || len(m.Shapes) != len(models[0].Shapes) {
				t.Fatalf("round %d: copy %d has score %v and %d shapes, copy 0 has %v and %d",
					round, i+1, m.Score, len(m.Shapes), models[0].Score, len(models[0].Shapes))
			}
		}
	}
	svg := models[0].SVG()
	for i, m := range models[1:] {
		if m.SVG() != svg {
			t.Fatalf("copy %d drew a different picture", i+1)
		}
	}
	if n := len(models[0].Shapes); n > cap {
		t.Fatalf("%d shapes, the cap is %d", n, cap)
	}
}

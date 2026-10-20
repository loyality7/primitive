package tests

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"

	"github.com/loyality7/primitive/primitive"
	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/rasterize"
	"github.com/loyality7/primitive/primitive/shape"
)

func TestShapeCodecRoundTrip(t *testing.T) {
	env := shape.NewEnv(64, 64, rand.New(rand.NewSource(3)))
	cases := map[shape.Type]func() shape.Shape{
		shape.TypeTriangle:         func() shape.Shape { return shape.NewRandomTriangle(env) },
		shape.TypeRectangle:        func() shape.Shape { return shape.NewRandomRectangle(env) },
		shape.TypeEllipse:          func() shape.Shape { return shape.NewRandomEllipse(env) },
		shape.TypeCircle:           func() shape.Shape { return shape.NewRandomCircle(env) },
		shape.TypeRotatedRectangle: func() shape.Shape { return shape.NewRandomRotatedRectangle(env) },
		shape.TypeQuadratic:        func() shape.Shape { return shape.NewRandomQuadratic(env) },
		shape.TypeRotatedEllipse:   func() shape.Shape { return shape.NewRandomRotatedEllipse(env) },
		shape.TypePolygon:          func() shape.Shape { return shape.NewRandomPolygon(env, 4, false) },
	}
	for typ, make := range cases {
		for i := 0; i < 20; i++ {
			s := make()
			s.Mutate() // mutated shapes are what the search actually finds
			gotType, params := shape.Encode(s)
			if gotType != typ {
				t.Fatalf("type %d encoded as %d", typ, gotType)
			}
			back, err := shape.Decode(env, gotType, params)
			if err != nil {
				t.Fatalf("type %d: %v", typ, err)
			}
			// Rasterize reuses one buffer, so copy the first result before the second call
			want := append([]rasterize.Scanline(nil), s.Rasterize()...)
			got := back.Rasterize()
			if !slices.Equal(want, got) { // slices.Equal treats nil and empty alike: a thin curve can cover no pixels
				t.Fatalf("type %d: the decoded shape covers different pixels", typ)
			}
			if _, again := shape.Encode(back); !reflect.DeepEqual(again, params) {
				t.Fatalf("type %d: encode is not stable", typ)
			}
		}
	}
}

func TestDecodeRejectsBadNumbers(t *testing.T) {
	env := shape.NewEnv(32, 32, rand.New(rand.NewSource(1)))
	bad := []struct {
		typ shape.Type
		p   []float64
	}{
		{shape.TypeTriangle, []float64{1, 2, 3}},
		{shape.TypeQuadratic, nil},
		{shape.TypePolygon, []float64{1, 0, 5, 5}},       // order 1
		{shape.TypePolygon, []float64{4, 0, 1, 2, 3, 4}}, // too few coordinates
		{shape.Type(99), []float64{1}},
	}
	for _, b := range bad {
		if _, err := shape.Decode(env, b.typ, b.p); err == nil {
			t.Errorf("Decode(%d, %v) should fail", b.typ, b.p)
		}
	}
}

// Several models of one picture search on their own and all add the best
// shape. They must stay exactly the same, or the parallel browser build would
// drift apart.
func TestSearchingModelsStayIdentical(t *testing.T) {
	bg := colors.Color{R: 128, G: 128, B: 128, A: 255}
	models := []*primitive.Model{
		primitive.NewModel(gradientImage(), bg, 48, 1, 11),
		primitive.NewModel(gradientImage(), bg, 48, 1, 22),
		primitive.NewModel(gradientImage(), bg, 48, 1, 33),
	}
	for round := 0; round < 8; round++ {
		var best *primitive.State
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
		}
		for i, m := range models[1:] {
			if m.Score != models[0].Score {
				t.Fatalf("round %d: model %d has score %v, model 0 has %v", round, i+1, m.Score, models[0].Score)
			}
		}
	}
	svg := models[0].SVG()
	for i, m := range models[1:] {
		if m.SVG() != svg {
			t.Fatalf("model %d drew a different picture", i+1)
		}
	}
	if models[0].Score >= 0.5 {
		t.Fatalf("score %v: the shared search did not improve the picture", models[0].Score)
	}
}

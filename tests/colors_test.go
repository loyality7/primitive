package tests

import (
	"testing"

	"github.com/loyality7/primitive/primitive/colors"
)

func TestSnap(t *testing.T) {
	palette := []colors.Color{{R: 0, G: 0, B: 0, A: 255}, {R: 255, G: 0, B: 0, A: 255}, {R: 255, G: 255, B: 255, A: 255}}
	cases := []struct{ in, want colors.Color }{
		{colors.Color{R: 10, G: 10, B: 10, A: 77}, colors.Color{R: 0, G: 0, B: 0, A: 77}},      // alpha is kept
		{colors.Color{R: 240, G: 30, B: 20, A: 128}, colors.Color{R: 255, G: 0, B: 0, A: 128}}, // nearest is red
		{colors.Color{R: 200, G: 200, B: 200, A: 1}, colors.Color{R: 255, G: 255, B: 255, A: 1}},
	}
	for _, c := range cases {
		if got := colors.Snap(c.in, palette); got != c.want {
			t.Errorf("Snap(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if in := (colors.Color{R: 1, G: 2, B: 3, A: 4}); colors.Snap(in, nil) != in {
		t.Error("empty palette must leave colors untouched")
	}
}

func TestParsePalette(t *testing.T) {
	p, err := colors.ParsePalette("#fff, #000000")
	if err != nil || len(p) != 2 || p[0] != (colors.Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("got %v, %v", p, err)
	}
	if p, err := colors.ParsePalette(""); p != nil || err != nil {
		t.Fatal("empty string must mean no palette")
	}
	for _, bad := range []string{"zzz", "#12", "#fff,#gg0000"} {
		if _, err := colors.ParsePalette(bad); err == nil {
			t.Errorf("ParsePalette(%q) should fail", bad)
		}
	}
}

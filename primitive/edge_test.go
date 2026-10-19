package primitive

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// left half flat grey, right half vertical stripes
func stripedImage() *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.RGBA{128, 128, 128, 255}
			if x >= 32 && x%2 == 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

func TestEdgeMapFavoursDetail(t *testing.T) {
	e := NewEdgeMap(stripedImage())
	rnd := rand.New(rand.NewSource(1))
	right := 0
	const n = 20000
	for i := 0; i < n; i++ {
		x, y := e.Sample(rnd)
		if x < 0 || x >= 64 || y < 0 || y >= 64 {
			t.Fatalf("sample out of bounds: %d,%d", x, y)
		}
		if x >= 31 {
			right++
		}
	}
	if frac := float64(right) / n; frac < 0.9 {
		t.Fatalf("only %.2f of samples in the detailed half, want >= 0.9", frac)
	}
}

func TestRandomPointUniformWhenOff(t *testing.T) {
	w := NewWorker(stripedImage(), 1)
	rnd := rand.New(rand.NewSource(9))
	w.Rnd = rand.New(rand.NewSource(9))
	for i := 0; i < 100; i++ {
		x, y := w.RandomPoint()
		wx := rnd.Intn(64)
		wy := rnd.Intn(64)
		if x != wx || y != wy {
			t.Fatalf("edge off must match plain Intn draws, got %d,%d want %d,%d", x, y, wx, wy)
		}
	}
}

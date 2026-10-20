package tests

import (
	"image"
	"testing"

	"github.com/loyality7/primitive/primitive/lowpoly"
)

func TestLowpolyCoversThePicture(t *testing.T) {
	m := lowpoly.New(80, 60, 160, 300)
	m.Border = 0
	frame := solid(80, 60, 200)
	if err := m.SetFrame(frame, 0); err != nil {
		t.Fatal(err)
	}
	if m.Triangles() < 100 {
		t.Fatalf("only %d triangles", m.Triangles())
	}
	out := m.Image().(*image.RGBA)
	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 200 || out.Pix[i+1] != 200 || out.Pix[i+2] != 200 {
			t.Fatalf("pixel %d is %d,%d,%d: the mesh left a hole", i/4, out.Pix[i], out.Pix[i+1], out.Pix[i+2])
		}
	}
}

func TestLowpolyFollowsTheFrame(t *testing.T) {
	m := lowpoly.New(80, 60, 160, 600)
	m.Border = 0
	frame := image.NewRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			o := frame.PixOffset(x, y)
			if x < 40 {
				frame.Pix[o] = 250 // left half red
			} else {
				frame.Pix[o+2] = 250 // right half blue
			}
			frame.Pix[o+3] = 255
		}
	}
	m.SetFrame(frame, 0)
	out := m.Image().(*image.RGBA)
	left, right := out.PixOffset(20, 60), out.PixOffset(140, 60)
	if out.Pix[left] < 200 || out.Pix[left+2] > 60 {
		t.Fatalf("left should be red, got %v", out.Pix[left:left+3])
	}
	if out.Pix[right+2] < 200 || out.Pix[right] > 60 {
		t.Fatalf("right should be blue, got %v", out.Pix[right:right+3])
	}
	if err := m.SetFrame(solid(10, 10, 1), 0); err == nil {
		t.Fatal("a frame of another size should be an error")
	}
	svg := m.SVG()
	if len(svg) < 500 {
		t.Fatal("svg looks empty")
	}
}

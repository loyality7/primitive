package shape

import (
	"fmt"
	"math"

	"github.com/fogleman/gg"

	"github.com/loyality7/primitive/primitive/internal/num"
	"github.com/loyality7/primitive/primitive/rasterize"
)

type Triangle struct {
	Env    *Env
	X1, Y1 int
	X2, Y2 int
	X3, Y3 int
}

func NewRandomTriangle(env *Env) *Triangle {
	rnd := env.Rnd
	x1, y1 := env.RandomPoint()
	x2 := x1 + rnd.Intn(31) - 15
	y2 := y1 + rnd.Intn(31) - 15
	x3 := x1 + rnd.Intn(31) - 15
	y3 := y1 + rnd.Intn(31) - 15
	t := &Triangle{env, x1, y1, x2, y2, x3, y3}
	t.Mutate()
	return t
}

func (t *Triangle) Draw(dc *gg.Context, scale float64) {
	dc.LineTo(float64(t.X1), float64(t.Y1))
	dc.LineTo(float64(t.X2), float64(t.Y2))
	dc.LineTo(float64(t.X3), float64(t.Y3))
	dc.ClosePath()
	dc.Fill()
}

func (t *Triangle) SVG(attrs string) string {
	return fmt.Sprintf(
		"<polygon %s points=\"%d,%d %d,%d %d,%d\" />",
		attrs, t.X1, t.Y1, t.X2, t.Y2, t.X3, t.Y3)
}

func (t *Triangle) Copy() Shape {
	a := *t
	return &a
}

func (t *Triangle) Mutate() {
	w := t.Env.W
	h := t.Env.H
	rnd := t.Env.Rnd
	const m = 16
	for {
		switch rnd.Intn(3) {
		case 0:
			t.X1 = num.ClampInt(t.X1+int(rnd.NormFloat64()*16), -m, w-1+m)
			t.Y1 = num.ClampInt(t.Y1+int(rnd.NormFloat64()*16), -m, h-1+m)
		case 1:
			t.X2 = num.ClampInt(t.X2+int(rnd.NormFloat64()*16), -m, w-1+m)
			t.Y2 = num.ClampInt(t.Y2+int(rnd.NormFloat64()*16), -m, h-1+m)
		case 2:
			t.X3 = num.ClampInt(t.X3+int(rnd.NormFloat64()*16), -m, w-1+m)
			t.Y3 = num.ClampInt(t.Y3+int(rnd.NormFloat64()*16), -m, h-1+m)
		}
		if t.Valid() {
			break
		}
	}
}

func (t *Triangle) Valid() bool {
	const minDegrees = 15
	var a1, a2, a3 float64
	{
		x1 := float64(t.X2 - t.X1)
		y1 := float64(t.Y2 - t.Y1)
		x2 := float64(t.X3 - t.X1)
		y2 := float64(t.Y3 - t.Y1)
		d1 := math.Sqrt(x1*x1 + y1*y1)
		d2 := math.Sqrt(x2*x2 + y2*y2)
		x1 /= d1
		y1 /= d1
		x2 /= d2
		y2 /= d2
		a1 = num.Degrees(math.Acos(x1*x2 + y1*y2))
	}
	{
		x1 := float64(t.X1 - t.X2)
		y1 := float64(t.Y1 - t.Y2)
		x2 := float64(t.X3 - t.X2)
		y2 := float64(t.Y3 - t.Y2)
		d1 := math.Sqrt(x1*x1 + y1*y1)
		d2 := math.Sqrt(x2*x2 + y2*y2)
		x1 /= d1
		y1 /= d1
		x2 /= d2
		y2 /= d2
		a2 = num.Degrees(math.Acos(x1*x2 + y1*y2))
	}
	a3 = 180 - a1 - a2
	return a1 > minDegrees && a2 > minDegrees && a3 > minDegrees
}

func (t *Triangle) Rasterize() []rasterize.Scanline {
	buf := t.Env.Lines[:0]
	lines := rasterizeTriangle(t.X1, t.Y1, t.X2, t.Y2, t.X3, t.Y3, buf)
	return rasterize.Crop(lines, t.Env.W, t.Env.H)
}

func rasterizeTriangle(x1, y1, x2, y2, x3, y3 int, buf []rasterize.Scanline) []rasterize.Scanline {
	if y1 > y3 {
		x1, x3 = x3, x1
		y1, y3 = y3, y1
	}
	if y1 > y2 {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}
	if y2 > y3 {
		x2, x3 = x3, x2
		y2, y3 = y3, y2
	}
	if y2 == y3 {
		return rasterizeTriangleBottom(x1, y1, x2, y2, x3, y3, buf)
	} else if y1 == y2 {
		return rasterizeTriangleTop(x1, y1, x2, y2, x3, y3, buf)
	} else {
		x4 := x1 + int((float64(y2-y1)/float64(y3-y1))*float64(x3-x1))
		y4 := y2
		buf = rasterizeTriangleBottom(x1, y1, x2, y2, x4, y4, buf)
		buf = rasterizeTriangleTop(x2, y2, x4, y4, x3, y3, buf)
		return buf
	}
}

func rasterizeTriangleBottom(x1, y1, x2, y2, x3, y3 int, buf []rasterize.Scanline) []rasterize.Scanline {
	s1 := float64(x2-x1) / float64(y2-y1)
	s2 := float64(x3-x1) / float64(y3-y1)
	ax := float64(x1)
	bx := float64(x1)
	for y := y1; y <= y2; y++ {
		a := int(ax)
		b := int(bx)
		ax += s1
		bx += s2
		if a > b {
			a, b = b, a
		}
		buf = append(buf, rasterize.Scanline{Y: y, X1: a, X2: b, Alpha: 0xffff})
	}
	return buf
}

func rasterizeTriangleTop(x1, y1, x2, y2, x3, y3 int, buf []rasterize.Scanline) []rasterize.Scanline {
	s1 := float64(x3-x1) / float64(y3-y1)
	s2 := float64(x3-x2) / float64(y3-y2)
	ax := float64(x3)
	bx := float64(x3)
	for y := y3; y > y1; y-- {
		ax -= s1
		bx -= s2
		a := int(ax)
		b := int(bx)
		if a > b {
			a, b = b, a
		}
		buf = append(buf, rasterize.Scanline{Y: y, X1: a, X2: b, Alpha: 0xffff})
	}
	return buf
}

package shape

import (
	"fmt"
	"math"

	"github.com/fogleman/gg"

	"github.com/loyality7/primitive/primitive/internal/num"
	"github.com/loyality7/primitive/primitive/rasterize"
)

type Rectangle struct {
	Env    *Env
	X1, Y1 int
	X2, Y2 int
}

func NewRandomRectangle(env *Env) *Rectangle {
	rnd := env.Rnd
	x1, y1 := env.RandomPoint()
	x2 := num.ClampInt(x1+rnd.Intn(32)+1, 0, env.W-1)
	y2 := num.ClampInt(y1+rnd.Intn(32)+1, 0, env.H-1)
	return &Rectangle{env, x1, y1, x2, y2}
}

func (r *Rectangle) bounds() (x1, y1, x2, y2 int) {
	x1, y1 = r.X1, r.Y1
	x2, y2 = r.X2, r.Y2
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	return
}

func (r *Rectangle) Draw(dc *gg.Context, scale float64) {
	x1, y1, x2, y2 := r.bounds()
	dc.DrawRectangle(float64(x1), float64(y1), float64(x2-x1+1), float64(y2-y1+1))
	dc.Fill()
}

func (r *Rectangle) SVG(attrs string) string {
	x1, y1, x2, y2 := r.bounds()
	w := x2 - x1 + 1
	h := y2 - y1 + 1
	return fmt.Sprintf(
		"<rect %s x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" />",
		attrs, x1, y1, w, h)
}

func (r *Rectangle) Copy() Shape {
	a := *r
	return &a
}

func (r *Rectangle) Mutate() {
	w := r.Env.W
	h := r.Env.H
	rnd := r.Env.Rnd
	switch rnd.Intn(2) {
	case 0:
		r.X1 = num.ClampInt(r.X1+int(rnd.NormFloat64()*16), 0, w-1)
		r.Y1 = num.ClampInt(r.Y1+int(rnd.NormFloat64()*16), 0, h-1)
	case 1:
		r.X2 = num.ClampInt(r.X2+int(rnd.NormFloat64()*16), 0, w-1)
		r.Y2 = num.ClampInt(r.Y2+int(rnd.NormFloat64()*16), 0, h-1)
	}
}

func (r *Rectangle) Rasterize() []rasterize.Scanline {
	x1, y1, x2, y2 := r.bounds()
	lines := r.Env.Lines[:0]
	for y := y1; y <= y2; y++ {
		lines = append(lines, rasterize.Scanline{Y: y, X1: x1, X2: x2, Alpha: 0xffff})
	}
	return lines
}

type RotatedRectangle struct {
	Env    *Env
	X, Y   int
	Sx, Sy int
	Angle  int
}

func NewRandomRotatedRectangle(env *Env) *RotatedRectangle {
	rnd := env.Rnd
	x, y := env.RandomPoint()
	sx := rnd.Intn(32) + 1
	sy := rnd.Intn(32) + 1
	a := rnd.Intn(360)
	r := &RotatedRectangle{env, x, y, sx, sy, a}
	r.Mutate()
	return r
}

func (r *RotatedRectangle) Draw(dc *gg.Context, scale float64) {
	sx, sy := float64(r.Sx), float64(r.Sy)
	dc.Push()
	dc.Translate(float64(r.X), float64(r.Y))
	dc.Rotate(num.Radians(float64(r.Angle)))
	dc.DrawRectangle(-sx/2, -sy/2, sx, sy)
	dc.Pop()
	dc.Fill()
}

func (r *RotatedRectangle) SVG(attrs string) string {
	return fmt.Sprintf(
		"<g transform=\"translate(%d %d) rotate(%d) scale(%d %d)\"><rect %s x=\"-0.5\" y=\"-0.5\" width=\"1\" height=\"1\" /></g>",
		r.X, r.Y, r.Angle, r.Sx, r.Sy, attrs)
}

func (r *RotatedRectangle) Copy() Shape {
	a := *r
	return &a
}

func (r *RotatedRectangle) Mutate() {
	w := r.Env.W
	h := r.Env.H
	rnd := r.Env.Rnd
	switch rnd.Intn(3) {
	case 0:
		r.X = num.ClampInt(r.X+int(rnd.NormFloat64()*16), 0, w-1)
		r.Y = num.ClampInt(r.Y+int(rnd.NormFloat64()*16), 0, h-1)
	case 1:
		r.Sx = num.ClampInt(r.Sx+int(rnd.NormFloat64()*16), 1, w-1)
		r.Sy = num.ClampInt(r.Sy+int(rnd.NormFloat64()*16), 1, h-1)
	case 2:
		r.Angle = r.Angle + int(rnd.NormFloat64()*32)
	}
	// for !r.Valid() {
	// 	r.Sx = num.ClampInt(r.Sx+int(rnd.NormFloat64()*16), 0, w-1)
	// 	r.Sy = num.ClampInt(r.Sy+int(rnd.NormFloat64()*16), 0, h-1)
	// }
}

func (r *RotatedRectangle) Valid() bool {
	a, b := r.Sx, r.Sy
	if a < b {
		a, b = b, a
	}
	aspect := float64(a) / float64(b)
	return aspect <= 5
}

func (r *RotatedRectangle) Rasterize() []rasterize.Scanline {
	w := r.Env.W
	h := r.Env.H
	sx, sy := float64(r.Sx), float64(r.Sy)
	angle := num.Radians(float64(r.Angle))
	rx1, ry1 := num.Rotate(-sx/2, -sy/2, angle)
	rx2, ry2 := num.Rotate(sx/2, -sy/2, angle)
	rx3, ry3 := num.Rotate(sx/2, sy/2, angle)
	rx4, ry4 := num.Rotate(-sx/2, sy/2, angle)
	x1, y1 := int(rx1)+r.X, int(ry1)+r.Y
	x2, y2 := int(rx2)+r.X, int(ry2)+r.Y
	x3, y3 := int(rx3)+r.X, int(ry3)+r.Y
	x4, y4 := int(rx4)+r.X, int(ry4)+r.Y
	miny := num.MinInt(y1, num.MinInt(y2, num.MinInt(y3, y4)))
	maxy := num.MaxInt(y1, num.MaxInt(y2, num.MaxInt(y3, y4)))
	n := maxy - miny + 1
	min := make([]int, n)
	max := make([]int, n)
	for i := range min {
		min[i] = w
	}
	xs := []int{x1, x2, x3, x4, x1}
	ys := []int{y1, y2, y3, y4, y1}
	// TODO: this could be better probably
	for i := 0; i < 4; i++ {
		x, y := float64(xs[i]), float64(ys[i])
		dx, dy := float64(xs[i+1]-xs[i]), float64(ys[i+1]-ys[i])
		count := int(math.Sqrt(dx*dx+dy*dy)) * 2
		for j := 0; j < count; j++ {
			t := float64(j) / float64(count-1)
			xi := int(x + dx*t)
			yi := int(y+dy*t) - miny
			min[yi] = num.MinInt(min[yi], xi)
			max[yi] = num.MaxInt(max[yi], xi)
		}
	}
	lines := r.Env.Lines[:0]
	for i := 0; i < n; i++ {
		y := miny + i
		if y < 0 || y >= h {
			continue
		}
		a := num.MaxInt(min[i], 0)
		b := num.MinInt(max[i], w-1)
		if b >= a {
			lines = append(lines, rasterize.Scanline{Y: y, X1: a, X2: b, Alpha: 0xffff})
		}
	}
	return lines
}

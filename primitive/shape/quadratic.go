package shape

import (
	"fmt"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/raster"

	"github.com/loyality7/primitive/primitive/internal/num"
	"github.com/loyality7/primitive/primitive/rasterize"
)

type Quadratic struct {
	Env    *Env
	X1, Y1 float64
	X2, Y2 float64
	X3, Y3 float64
	Width  float64
}

func NewRandomQuadratic(env *Env) *Quadratic {
	rnd := env.Rnd
	x1, y1 := env.RandomPointF()
	x2 := x1 + rnd.Float64()*40 - 20
	y2 := y1 + rnd.Float64()*40 - 20
	x3 := x2 + rnd.Float64()*40 - 20
	y3 := y2 + rnd.Float64()*40 - 20
	width := 1.0 / 2
	q := &Quadratic{env, x1, y1, x2, y2, x3, y3, width}
	q.Mutate()
	return q
}

func (q *Quadratic) Draw(dc *gg.Context, scale float64) {
	dc.MoveTo(q.X1, q.Y1)
	dc.QuadraticTo(q.X2, q.Y2, q.X3, q.Y3)
	dc.SetLineWidth(q.Width * scale)
	dc.Stroke()
}

func (q *Quadratic) SVG(attrs string) string {
	// TODO: this is a little silly
	attrs = strings.Replace(attrs, "fill", "stroke", -1)
	return fmt.Sprintf(
		"<path %s fill=\"none\" d=\"M %f %f Q %f %f, %f %f\" stroke-width=\"%f\" />",
		attrs, q.X1, q.Y1, q.X2, q.Y2, q.X3, q.Y3, q.Width)
}

func (q *Quadratic) Copy() Shape {
	a := *q
	return &a
}

func (q *Quadratic) Mutate() {
	const m = 16
	w := q.Env.W
	h := q.Env.H
	rnd := q.Env.Rnd
	for {
		switch rnd.Intn(4) {
		case 0:
			q.X1 = num.Clamp(q.X1+rnd.NormFloat64()*16, -m, float64(w-1+m))
			q.Y1 = num.Clamp(q.Y1+rnd.NormFloat64()*16, -m, float64(h-1+m))
		case 1:
			q.X2 = num.Clamp(q.X2+rnd.NormFloat64()*16, -m, float64(w-1+m))
			q.Y2 = num.Clamp(q.Y2+rnd.NormFloat64()*16, -m, float64(h-1+m))
		case 2:
			q.X3 = num.Clamp(q.X3+rnd.NormFloat64()*16, -m, float64(w-1+m))
			q.Y3 = num.Clamp(q.Y3+rnd.NormFloat64()*16, -m, float64(h-1+m))
		case 3:
			q.Width = num.Clamp(q.Width+rnd.NormFloat64(), 1, 16)
		}
		if q.Valid() {
			break
		}
	}
}

func (q *Quadratic) Valid() bool {
	dx12 := int(q.X1 - q.X2)
	dy12 := int(q.Y1 - q.Y2)
	dx23 := int(q.X2 - q.X3)
	dy23 := int(q.Y2 - q.Y3)
	dx13 := int(q.X1 - q.X3)
	dy13 := int(q.Y1 - q.Y3)
	d12 := dx12*dx12 + dy12*dy12
	d23 := dx23*dx23 + dy23*dy23
	d13 := dx13*dx13 + dy13*dy13
	return d13 > d12 && d13 > d23
}

func (q *Quadratic) Rasterize() []rasterize.Scanline {
	var path raster.Path
	p1 := rasterize.Fixp(q.X1, q.Y1)
	p2 := rasterize.Fixp(q.X2, q.Y2)
	p3 := rasterize.Fixp(q.X3, q.Y3)
	path.Start(p1)
	path.Add2(p2, p3)
	width := rasterize.Fix(q.Width)
	return q.Env.Stroke(path, width, raster.RoundCapper, raster.RoundJoiner)
}

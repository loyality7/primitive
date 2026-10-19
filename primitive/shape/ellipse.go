package shape

import (
	"fmt"
	"math"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/raster"

	"github.com/loyality7/primitive/primitive/internal/num"
	"github.com/loyality7/primitive/primitive/rasterize"
)

type Ellipse struct {
	Env    *Env
	X, Y   int
	Rx, Ry int
	Circle bool
}

func NewRandomEllipse(env *Env) *Ellipse {
	rnd := env.Rnd
	x, y := env.RandomPoint()
	rx := rnd.Intn(32) + 1
	ry := rnd.Intn(32) + 1
	return &Ellipse{env, x, y, rx, ry, false}
}

func NewRandomCircle(env *Env) *Ellipse {
	rnd := env.Rnd
	x, y := env.RandomPoint()
	r := rnd.Intn(32) + 1
	return &Ellipse{env, x, y, r, r, true}
}

func (c *Ellipse) Draw(dc *gg.Context, scale float64) {
	dc.DrawEllipse(float64(c.X), float64(c.Y), float64(c.Rx), float64(c.Ry))
	dc.Fill()
}

func (c *Ellipse) SVG(attrs string) string {
	return fmt.Sprintf(
		"<ellipse %s cx=\"%d\" cy=\"%d\" rx=\"%d\" ry=\"%d\" />",
		attrs, c.X, c.Y, c.Rx, c.Ry)
}

func (c *Ellipse) Copy() Shape {
	a := *c
	return &a
}

func (c *Ellipse) Mutate() {
	w := c.Env.W
	h := c.Env.H
	rnd := c.Env.Rnd
	switch rnd.Intn(3) {
	case 0:
		c.X = num.ClampInt(c.X+int(rnd.NormFloat64()*16), 0, w-1)
		c.Y = num.ClampInt(c.Y+int(rnd.NormFloat64()*16), 0, h-1)
	case 1:
		c.Rx = num.ClampInt(c.Rx+int(rnd.NormFloat64()*16), 1, w-1)
		if c.Circle {
			c.Ry = c.Rx
		}
	case 2:
		c.Ry = num.ClampInt(c.Ry+int(rnd.NormFloat64()*16), 1, h-1)
		if c.Circle {
			c.Rx = c.Ry
		}
	}
}

func (c *Ellipse) Rasterize() []rasterize.Scanline {
	w := c.Env.W
	h := c.Env.H
	lines := c.Env.Lines[:0]
	aspect := float64(c.Rx) / float64(c.Ry)
	for dy := 0; dy < c.Ry; dy++ {
		y1 := c.Y - dy
		y2 := c.Y + dy
		if (y1 < 0 || y1 >= h) && (y2 < 0 || y2 >= h) {
			continue
		}
		s := int(math.Sqrt(float64(c.Ry*c.Ry-dy*dy)) * aspect)
		x1 := c.X - s
		x2 := c.X + s
		if x1 < 0 {
			x1 = 0
		}
		if x2 >= w {
			x2 = w - 1
		}
		if y1 >= 0 && y1 < h {
			lines = append(lines, rasterize.Scanline{Y: y1, X1: x1, X2: x2, Alpha: 0xffff})
		}
		if y2 >= 0 && y2 < h && dy > 0 {
			lines = append(lines, rasterize.Scanline{Y: y2, X1: x1, X2: x2, Alpha: 0xffff})
		}
	}
	return lines
}

type RotatedEllipse struct {
	Env    *Env
	X, Y   float64
	Rx, Ry float64
	Angle  float64
}

func NewRandomRotatedEllipse(env *Env) *RotatedEllipse {
	rnd := env.Rnd
	x, y := env.RandomPointF()
	rx := rnd.Float64()*32 + 1
	ry := rnd.Float64()*32 + 1
	a := rnd.Float64() * 360
	return &RotatedEllipse{env, x, y, rx, ry, a}
}

func (c *RotatedEllipse) Draw(dc *gg.Context, scale float64) {
	dc.Push()
	dc.RotateAbout(num.Radians(c.Angle), c.X, c.Y)
	dc.DrawEllipse(c.X, c.Y, c.Rx, c.Ry)
	dc.Fill()
	dc.Pop()
}

func (c *RotatedEllipse) SVG(attrs string) string {
	return fmt.Sprintf(
		"<g transform=\"translate(%f %f) rotate(%f) scale(%f %f)\"><ellipse %s cx=\"0\" cy=\"0\" rx=\"1\" ry=\"1\" /></g>",
		c.X, c.Y, c.Angle, c.Rx, c.Ry, attrs)
}

func (c *RotatedEllipse) Copy() Shape {
	a := *c
	return &a
}

func (c *RotatedEllipse) Mutate() {
	w := c.Env.W
	h := c.Env.H
	rnd := c.Env.Rnd
	switch rnd.Intn(3) {
	case 0:
		c.X = num.Clamp(c.X+rnd.NormFloat64()*16, 0, float64(w-1))
		c.Y = num.Clamp(c.Y+rnd.NormFloat64()*16, 0, float64(h-1))
	case 1:
		c.Rx = num.Clamp(c.Rx+rnd.NormFloat64()*16, 1, float64(w-1))
		c.Ry = num.Clamp(c.Ry+rnd.NormFloat64()*16, 1, float64(w-1))
	case 2:
		c.Angle = c.Angle + rnd.NormFloat64()*32
	}
}

func (c *RotatedEllipse) Rasterize() []rasterize.Scanline {
	var path raster.Path
	const n = 16
	for i := 0; i < n; i++ {
		p1 := float64(i+0) / n
		p2 := float64(i+1) / n
		a1 := p1 * 2 * math.Pi
		a2 := p2 * 2 * math.Pi
		x0 := c.Rx * math.Cos(a1)
		y0 := c.Ry * math.Sin(a1)
		x1 := c.Rx * math.Cos(a1+(a2-a1)/2)
		y1 := c.Ry * math.Sin(a1+(a2-a1)/2)
		x2 := c.Rx * math.Cos(a2)
		y2 := c.Ry * math.Sin(a2)
		cx := 2*x1 - x0/2 - x2/2
		cy := 2*y1 - y0/2 - y2/2
		x0, y0 = num.Rotate(x0, y0, num.Radians(c.Angle))
		cx, cy = num.Rotate(cx, cy, num.Radians(c.Angle))
		x2, y2 = num.Rotate(x2, y2, num.Radians(c.Angle))
		if i == 0 {
			path.Start(rasterize.Fixp(x0+c.X, y0+c.Y))
		}
		path.Add2(rasterize.Fixp(cx+c.X, cy+c.Y), rasterize.Fixp(x2+c.X, y2+c.Y))
	}
	return c.Env.Fill(path)
}

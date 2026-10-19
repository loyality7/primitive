package rasterize

import (
	"github.com/golang/freetype/raster"
	"golang.org/x/image/math/fixed"
)

// Fix converts to 26.6 fixed point.
func Fix(x float64) fixed.Int26_6 {
	return fixed.Int26_6(x * 64)
}

func Fixp(x, y float64) fixed.Point26_6 {
	return fixed.Point26_6{X: Fix(x), Y: Fix(y)}
}

type painter struct {
	Lines []Scanline
}

func (p *painter) Paint(spans []raster.Span, done bool) {
	for _, span := range spans {
		p.Lines = append(p.Lines, Scanline{span.Y, span.X0, span.X1 - 1, span.Alpha})
	}
}

// FillPath rasterises a filled path into buf (reused between calls).
func FillPath(r *raster.Rasterizer, buf []Scanline, path raster.Path) []Scanline {
	r.Clear()
	r.UseNonZeroWinding = true
	r.AddPath(path)
	var p painter
	p.Lines = buf[:0]
	r.Rasterize(&p)
	return p.Lines
}

// StrokePath rasterises a stroked path into buf (reused between calls).
func StrokePath(r *raster.Rasterizer, buf []Scanline, path raster.Path, width fixed.Int26_6, cr raster.Capper, jr raster.Joiner) []Scanline {
	r.Clear()
	r.UseNonZeroWinding = true
	r.AddStroke(path, width, cr, jr)
	var p painter
	p.Lines = buf[:0]
	r.Rasterize(&p)
	return p.Lines
}

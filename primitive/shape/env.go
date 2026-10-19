package shape

import (
	"math/rand"

	"github.com/golang/freetype/raster"
	"golang.org/x/image/math/fixed"

	"github.com/loyality7/primitive/primitive/rasterize"
)

// Env is what shapes need from the engine: image size, a random source, a
// rasteriser with a reusable scanline buffer, and optional edge-weighted
// placement. A worker owns one Env.
type Env struct {
	W, H       int
	Rnd        *rand.Rand
	Rasterizer *raster.Rasterizer
	Lines      []rasterize.Scanline
	Edge       *EdgeMap // nil = uniform placement
	EdgeBias   float64  // chance a new shape is anchored by Edge instead of uniformly
}

func NewEnv(w, h int, rnd *rand.Rand) *Env {
	return &Env{
		W: w, H: h, Rnd: rnd,
		Rasterizer: raster.NewRasterizer(w, h),
		Lines:      make([]rasterize.Scanline, 0, 4096), // TODO: based on height
	}
}

// RandomPoint picks where a new random shape is anchored. With no edge map it
// is uniform and consumes the RNG exactly as before.
func (e *Env) RandomPoint() (int, int) {
	rnd := e.Rnd
	if e.Edge != nil && rnd.Float64() < e.EdgeBias {
		return e.Edge.Sample(rnd)
	}
	x := rnd.Intn(e.W)
	return x, rnd.Intn(e.H)
}

func (e *Env) RandomPointF() (float64, float64) {
	rnd := e.Rnd
	if e.Edge != nil && rnd.Float64() < e.EdgeBias {
		x, y := e.Edge.Sample(rnd)
		return float64(x) + rnd.Float64(), float64(y) + rnd.Float64()
	}
	x := rnd.Float64() * float64(e.W)
	return x, rnd.Float64() * float64(e.H)
}

// Fill rasterises a filled path using the Env's reusable buffer.
func (e *Env) Fill(path raster.Path) []rasterize.Scanline {
	return rasterize.FillPath(e.Rasterizer, e.Lines, path)
}

// Stroke rasterises a stroked path using the Env's reusable buffer.
func (e *Env) Stroke(path raster.Path, width fixed.Int26_6, cr raster.Capper, jr raster.Joiner) []rasterize.Scanline {
	return rasterize.StrokePath(e.Rasterizer, e.Lines, path, width, cr, jr)
}

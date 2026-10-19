package primitive

import (
	"image"
	"math"
	"math/rand"
	"sort"
)

// EdgeMap is a cumulative distribution over pixels weighted by local
// contrast (Sobel gradient of luma) of the target image. Read-only after
// creation, so workers share one.
type EdgeMap struct {
	W   int
	cdf []float64
}

func NewEdgeMap(target *image.RGBA) *EdgeMap {
	b := target.Bounds()
	w, h := b.Dx(), b.Dy()
	luma := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := target.PixOffset(x, y)
			luma[y*w+x] = 0.299*float64(target.Pix[i]) + 0.587*float64(target.Pix[i+1]) + 0.114*float64(target.Pix[i+2])
		}
	}
	at := func(x, y int) float64 {
		return luma[clampInt(y, 0, h-1)*w+clampInt(x, 0, w-1)]
	}
	cdf := make([]float64, w*h)
	var total float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gx := at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1) - at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1)
			gy := at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1) - at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1)
			total += math.Hypot(gx, gy) + 1 // +1: flat areas stay reachable
			cdf[y*w+x] = total
		}
	}
	return &EdgeMap{w, cdf}
}

func (e *EdgeMap) Sample(rnd *rand.Rand) (x, y int) {
	u := rnd.Float64() * e.cdf[len(e.cdf)-1]
	i := sort.SearchFloat64s(e.cdf, u)
	if i >= len(e.cdf) {
		i = len(e.cdf) - 1
	}
	return i % e.W, i / e.W
}

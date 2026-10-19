package shape

import (
	"github.com/fogleman/gg"

	"github.com/loyality7/primitive/primitive/rasterize"
)

type Shape interface {
	Rasterize() []rasterize.Scanline
	Copy() Shape
	Mutate()
	Draw(dc *gg.Context, scale float64)
	SVG(attrs string) string
}

// Type selects which kind of shape to draw.
type Type int

const (
	TypeAny Type = iota
	TypeTriangle
	TypeRectangle
	TypeEllipse
	TypeCircle
	TypeRotatedRectangle
	TypeQuadratic
	TypeRotatedEllipse
	TypePolygon
)

package shape

import "fmt"

// Encode flattens a shape into its type and a list of numbers. Decode turns
// them back into the same shape, possibly in another Env, so a shape found by
// one worker can be sent to others (or saved).
func Encode(s Shape) (Type, []float64) {
	f := func(v ...int) []float64 {
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = float64(x)
		}
		return out
	}
	switch v := s.(type) {
	case *Triangle:
		return TypeTriangle, f(v.X1, v.Y1, v.X2, v.Y2, v.X3, v.Y3)
	case *Rectangle:
		return TypeRectangle, f(v.X1, v.Y1, v.X2, v.Y2)
	case *Ellipse:
		if v.Circle {
			return TypeCircle, f(v.X, v.Y, v.Rx, v.Ry)
		}
		return TypeEllipse, f(v.X, v.Y, v.Rx, v.Ry)
	case *RotatedRectangle:
		return TypeRotatedRectangle, f(v.X, v.Y, v.Sx, v.Sy, v.Angle)
	case *Quadratic:
		return TypeQuadratic, []float64{v.X1, v.Y1, v.X2, v.Y2, v.X3, v.Y3, v.Width}
	case *RotatedEllipse:
		return TypeRotatedEllipse, []float64{v.X, v.Y, v.Rx, v.Ry, v.Angle}
	case *Polygon:
		convex := 0.0
		if v.Convex {
			convex = 1
		}
		out := append([]float64{float64(v.Order), convex}, v.X...)
		return TypePolygon, append(out, v.Y...)
	}
	panic(fmt.Sprintf("shape.Encode: unknown shape %T", s))
}

// Decode rebuilds a shape from the output of Encode.
func Decode(env *Env, t Type, p []float64) (Shape, error) {
	want := map[Type]int{
		TypeTriangle: 6, TypeRectangle: 4, TypeEllipse: 4, TypeCircle: 4,
		TypeRotatedRectangle: 5, TypeQuadratic: 7, TypeRotatedEllipse: 5,
	}
	if n, ok := want[t]; ok && len(p) != n {
		return nil, fmt.Errorf("shape type %d needs %d numbers, got %d", t, n, len(p))
	}
	i := func(k int) int { return int(p[k]) }
	switch t {
	case TypeTriangle:
		return &Triangle{env, i(0), i(1), i(2), i(3), i(4), i(5)}, nil
	case TypeRectangle:
		return &Rectangle{env, i(0), i(1), i(2), i(3)}, nil
	case TypeEllipse:
		return &Ellipse{env, i(0), i(1), i(2), i(3), false}, nil
	case TypeCircle:
		return &Ellipse{env, i(0), i(1), i(2), i(3), true}, nil
	case TypeRotatedRectangle:
		return &RotatedRectangle{env, i(0), i(1), i(2), i(3), i(4)}, nil
	case TypeQuadratic:
		return &Quadratic{env, p[0], p[1], p[2], p[3], p[4], p[5], p[6]}, nil
	case TypeRotatedEllipse:
		return &RotatedEllipse{env, p[0], p[1], p[2], p[3], p[4]}, nil
	case TypePolygon:
		if len(p) < 2 || int(p[0]) < 3 || len(p) != 2+2*int(p[0]) {
			return nil, fmt.Errorf("a polygon needs its order (3 or more) and 2 x order coordinates")
		}
		n := int(p[0])
		return &Polygon{env, n, p[1] == 1, append([]float64(nil), p[2:2+n]...), append([]float64(nil), p[2+n:]...)}, nil
	}
	return nil, fmt.Errorf("unknown shape type %d", t)
}

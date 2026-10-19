package rasterize

import "github.com/loyality7/primitive/primitive/internal/num"

type Scanline struct {
	Y, X1, X2 int
	Alpha     uint32
}

// Crop clips lines to a w x h image, dropping lines that fall outside.
func Crop(lines []Scanline, w, h int) []Scanline {
	i := 0
	for _, line := range lines {
		if line.Y < 0 || line.Y >= h {
			continue
		}
		if line.X1 >= w {
			continue
		}
		if line.X2 < 0 {
			continue
		}
		line.X1 = num.ClampInt(line.X1, 0, w-1)
		line.X2 = num.ClampInt(line.X2, 0, w-1)
		if line.X1 > line.X2 {
			continue
		}
		lines[i] = line
		i++
	}
	return lines[:i]
}

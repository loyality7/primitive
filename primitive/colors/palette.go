package colors

import (
	"fmt"
	"regexp"
	"strings"
)

var hexColor = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// ParsePalette parses a comma-separated list of #rgb / #rrggbb colors.
// An empty string gives a nil palette (no restriction).
func ParsePalette(s string) ([]Color, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var palette []Color
	for _, h := range strings.Split(s, ",") {
		h = strings.TrimSpace(h)
		if !hexColor.MatchString(h) {
			return nil, fmt.Errorf("invalid palette color %q (want #rgb or #rrggbb)", h)
		}
		palette = append(palette, MakeHexColor(h))
	}
	return palette, nil
}

// Snap returns the palette color nearest to c in RGB, keeping c's
// alpha. An empty palette leaves c untouched.
func Snap(c Color, palette []Color) Color {
	if len(palette) == 0 {
		return c
	}
	best, bestD := palette[0], -1
	for _, p := range palette {
		dr, dg, db := p.R-c.R, p.G-c.G, p.B-c.B
		if d := dr*dr + dg*dg + db*db; bestD < 0 || d < bestD {
			best, bestD = p, d
		}
	}
	best.A = c.A
	return best
}

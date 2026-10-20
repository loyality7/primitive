// Package lowpoly draws a picture as a mesh of triangles. The mesh is made once
// from a handful of points, more of them where the picture has detail, and
// after that only the triangle colors change from frame to frame. That makes it
// fast and steady enough for live video.
package lowpoly

import (
	"fmt"
	"image"
	"math"
	"math/rand"
	"strings"

	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/shape"
)

type point struct{ x, y float64 }

type tri struct {
	a, b, c int
	// circumcircle, for the triangulation
	cx, cy, r2 float64
}

// Model is a triangle mesh over a W x H picture.
type Model struct {
	W, H, Sw, Sh int
	// Border is the width in output pixels of the dark outline of each
	// triangle. 0 gives no outline.
	Border int
	// Palette, when set, snaps every drawn color to its nearest entry.
	Palette []colors.Color

	points  int
	rnd     *rand.Rand
	pts     []point
	tris    [][3]int
	col     [][3]float32 // smoothed color of each triangle
	have    bool         // the colors hold a frame
	last    *image.RGBA  // the latest frame, kept to re-place the points
	haveCol bool
}

// New makes a model for w x h frames with about the given number of points.
// size is the longer side of the output. The mesh starts as an even grid; the
// first frame moves the points to where the detail is.
func New(w, h, size, points int) *Model {
	m := &Model{W: w, H: h, Border: 1, points: max(points, 12), rnd: rand.New(rand.NewSource(1))}
	aspect := float64(w) / float64(h)
	if aspect >= 1 {
		m.Sw, m.Sh = size, int(float64(size)/aspect)
	} else {
		m.Sw, m.Sh = int(float64(size)*aspect), size
	}
	m.Sw, m.Sh = max(m.Sw, 1), max(m.Sh, 1)
	m.last = image.NewRGBA(image.Rect(0, 0, w, h))
	m.place(nil)
	return m
}

// place picks the points and builds the mesh. With a frame, 70 percent of the
// points follow its edges; the rest are an even jittered grid, so flat areas
// still get triangles.
func (m *Model) place(frame *image.RGBA) {
	// pixel x covers [x, x+1), so the picture spans -0.5 to W-0.5 in pixel-center units
	w, h := float64(m.W), float64(m.H)
	at := func(x, y float64) point { return point{x - 0.5, y - 0.5} }
	var pts []point
	// the four corners and points along the border keep the mesh covering the whole picture
	side := max(2, int(math.Sqrt(float64(m.points))/3))
	for i := 0; i <= side; i++ {
		t := float64(i) / float64(side)
		pts = append(pts, at(t*w, 0), at(t*w, h), at(0, t*h), at(w, t*h))
	}
	budget := m.points - len(pts)
	grid := budget
	if frame != nil {
		grid = budget * 3 / 10
	}
	// jittered grid
	cols := max(2, int(math.Sqrt(float64(grid)*float64(m.W)/float64(m.H))))
	rows := max(2, grid/cols)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			pts = append(pts, at(
				(float64(c)+0.15+0.7*m.rnd.Float64())/float64(cols)*w,
				(float64(r)+0.15+0.7*m.rnd.Float64())/float64(rows)*h,
			))
		}
	}
	if frame != nil {
		edges := shape.NewEdgeMap(frame)
		for i := 0; i < budget-grid; i++ {
			x, y := edges.Sample(m.rnd)
			pts = append(pts, point{float64(x) + m.rnd.Float64() - 0.5, float64(y) + m.rnd.Float64() - 0.5})
		}
	}
	m.pts = dedupe(pts)
	m.tris = triangulate(m.pts)
	m.col = make([][3]float32, len(m.tris))
	m.have = false
}

// dedupe drops points that are almost on top of each other, which the
// triangulation cannot use.
func dedupe(pts []point) []point {
	seen := map[[2]int]bool{}
	out := pts[:0:0]
	for _, p := range pts {
		k := [2]int{int(math.Round(p.x * 2)), int(math.Round(p.y * 2))}
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

func circum(p [3]point) (cx, cy, r2 float64, ok bool) {
	ax, ay, bx, by, cx2, cy2 := p[0].x, p[0].y, p[1].x, p[1].y, p[2].x, p[2].y
	d := 2 * (ax*(by-cy2) + bx*(cy2-ay) + cx2*(ay-by))
	if math.Abs(d) < 1e-12 {
		return 0, 0, 0, false
	}
	ux := ((ax*ax+ay*ay)*(by-cy2) + (bx*bx+by*by)*(cy2-ay) + (cx2*cx2+cy2*cy2)*(ay-by)) / d
	uy := ((ax*ax+ay*ay)*(cx2-bx) + (bx*bx+by*by)*(ax-cx2) + (cx2*cx2+cy2*cy2)*(bx-ax)) / d
	return ux, uy, (ax-ux)*(ax-ux) + (ay-uy)*(ay-uy), true
}

// triangulate is the Bowyer-Watson Delaunay triangulation: add the points one
// at a time, remove the triangles whose circle holds the new point, and join
// the hole to the point.
func triangulate(pts []point) [][3]int {
	n := len(pts)
	all := append(append([]point(nil), pts...),
		point{-1e5, -1e5}, point{1e5, -1e5}, point{0, 1e5}) // a triangle around everything
	var tris []tri
	add := func(a, b, c int) {
		if cx, cy, r2, ok := circum([3]point{all[a], all[b], all[c]}); ok {
			tris = append(tris, tri{a, b, c, cx, cy, r2})
		}
	}
	add(n, n+1, n+2)
	for i := 0; i < n; i++ {
		p := all[i]
		edges := map[[2]int]int{}
		kept := tris[:0]
		for _, t := range tris {
			if (p.x-t.cx)*(p.x-t.cx)+(p.y-t.cy)*(p.y-t.cy) < t.r2 {
				for _, e := range [][2]int{{t.a, t.b}, {t.b, t.c}, {t.c, t.a}} {
					if e[0] > e[1] {
						e[0], e[1] = e[1], e[0]
					}
					edges[e]++
				}
			} else {
				kept = append(kept, t)
			}
		}
		tris = kept
		for e, count := range edges {
			if count == 1 { // an edge of the hole
				add(e[0], e[1], i)
			}
		}
	}
	var out [][3]int
	for _, t := range tris {
		if t.a < n && t.b < n && t.c < n {
			out = append(out, [3]int{t.a, t.b, t.c})
		}
	}
	return out
}

// Triangles is the number of triangles in the mesh.
func (m *Model) Triangles() int { return len(m.tris) }

// Reposition moves the points to where the latest frame has detail.
func (m *Model) Reposition() {
	if m.haveCol {
		m.place(m.last)
	}
}

// SetFrame colors every triangle from the frame, which must be W x H. smooth
// (0 to 1) blends the colors with the earlier frames to calm flicker. The
// first frame also places the points where it has detail.
func (m *Model) SetFrame(img *image.RGBA, smooth float64) error {
	b := img.Bounds()
	if b.Dx() != m.W || b.Dy() != m.H {
		return fmt.Errorf("frame is %dx%d, the model is %dx%d", b.Dx(), b.Dy(), m.W, m.H)
	}
	smooth = math.Max(0, math.Min(smooth, 0.98))
	s32 := float32(smooth)
	if !m.haveCol {
		m.haveCol = true
		m.last = img
		m.place(img)
	}
	m.last = img
	px := func(x, y float64) (float32, float32, float32) {
		xi := max(0, min(m.W-1, int(x+0.5)))
		yi := max(0, min(m.H-1, int(y+0.5)))
		o := img.PixOffset(b.Min.X+xi, b.Min.Y+yi)
		return float32(img.Pix[o]), float32(img.Pix[o+1]), float32(img.Pix[o+2])
	}
	for i, t := range m.tris {
		a, bb, c := m.pts[t[0]], m.pts[t[1]], m.pts[t[2]]
		gx, gy := (a.x+bb.x+c.x)/3, (a.y+bb.y+c.y)/3
		var sr, sg, sb float32
		// the middle and three points half way from it to each corner
		for _, q := range [4]point{{gx, gy}, {(gx + a.x) / 2, (gy + a.y) / 2}, {(gx + bb.x) / 2, (gy + bb.y) / 2}, {(gx + c.x) / 2, (gy + c.y) / 2}} {
			r, g, bl := px(q.x, q.y)
			sr, sg, sb = sr+r, sg+g, sb+bl
		}
		want := [3]float32{sr / 4, sg / 4, sb / 4}
		if !m.have || smooth == 0 {
			m.col[i] = want
		} else {
			for k := 0; k < 3; k++ {
				m.col[i][k] = m.col[i][k]*s32 + want[k]*(1-s32)
			}
		}
	}
	m.have = true
	return nil
}

func (m *Model) rgb(i int) (uint8, uint8, uint8) {
	r, g, b := uint8(m.col[i][0]+0.5), uint8(m.col[i][1]+0.5), uint8(m.col[i][2]+0.5)
	if len(m.Palette) > 0 {
		c := colors.Snap(colors.Color{R: int(r), G: int(g), B: int(b), A: 255}, m.Palette)
		r, g, b = uint8(c.R), uint8(c.G), uint8(c.B)
	}
	return r, g, b
}

// RenderInto draws the mesh into dst, which must be Sw x Sh.
func (m *Model) RenderInto(dst *image.RGBA) {
	for i := range dst.Pix {
		dst.Pix[i] = 0
	}
	for i := 3; i < len(dst.Pix); i += 4 {
		dst.Pix[i] = 255
	}
	sx, sy := float64(m.Sw)/float64(m.W), float64(m.Sh)/float64(m.H)
	for i, t := range m.tris {
		r, g, b := m.rgb(i)
		p := [3]point{}
		for k := 0; k < 3; k++ {
			p[k] = point{(m.pts[t[k]].x + 0.5) * sx, (m.pts[t[k]].y + 0.5) * sy}
		}
		fillTriangle(dst, p, r, g, b)
	}
	if m.Border > 0 {
		for _, t := range m.tris {
			for k := 0; k < 3; k++ {
				a, bb := m.pts[t[k]], m.pts[t[(k+1)%3]]
				line(dst, (a.x+0.5)*sx, (a.y+0.5)*sy, (bb.x+0.5)*sx, (bb.y+0.5)*sy, m.Border)
			}
		}
	}
}

// fillTriangle paints the pixels whose centers are inside the triangle (or on
// its edge, so neighbours never leave a gap).
func fillTriangle(im *image.RGBA, p [3]point, r, g, b uint8) {
	minX := int(math.Floor(math.Min(p[0].x, math.Min(p[1].x, p[2].x))))
	maxX := int(math.Ceil(math.Max(p[0].x, math.Max(p[1].x, p[2].x))))
	minY := int(math.Floor(math.Min(p[0].y, math.Min(p[1].y, p[2].y))))
	maxY := int(math.Ceil(math.Max(p[0].y, math.Max(p[1].y, p[2].y))))
	minX, minY = max(minX, 0), max(minY, 0)
	maxX, maxY = min(maxX, im.Rect.Dx()-1), min(maxY, im.Rect.Dy()-1)
	area := (p[1].x-p[0].x)*(p[2].y-p[0].y) - (p[2].x-p[0].x)*(p[1].y-p[0].y)
	if area == 0 {
		return
	}
	const eps = -1e-9
	for y := minY; y <= maxY; y++ {
		cy := float64(y) + 0.5
		for x := minX; x <= maxX; x++ {
			cx := float64(x) + 0.5
			w0 := ((p[1].x-cx)*(p[2].y-cy) - (p[2].x-cx)*(p[1].y-cy)) / area
			w1 := ((p[2].x-cx)*(p[0].y-cy) - (p[0].x-cx)*(p[2].y-cy)) / area
			w2 := 1 - w0 - w1
			if w0 >= eps && w1 >= eps && w2 >= eps {
				o := im.PixOffset(x, y)
				im.Pix[o], im.Pix[o+1], im.Pix[o+2] = r, g, b
			}
		}
	}
}

// line draws a dark line of the given width.
func line(im *image.RGBA, x0, y0, x1, y1 float64, width int) {
	n := int(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
	half := width / 2
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, y := int(x0+(x1-x0)*t), int(y0+(y1-y0)*t)
		for dy := -half; dy <= (width-1)/2; dy++ {
			for dx := -half; dx <= (width-1)/2; dx++ {
				if px, py := x+dx, y+dy; px >= 0 && py >= 0 && px < im.Rect.Dx() && py < im.Rect.Dy() {
					o := im.PixOffset(px, py)
					im.Pix[o], im.Pix[o+1], im.Pix[o+2] = 0, 0, 0
				}
			}
		}
	}
}

// Image renders the current state.
func (m *Model) Image() image.Image {
	out := image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
	m.RenderInto(out)
	return out
}

// SVG renders the current state, one polygon per triangle.
func (m *Model) SVG() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" version=\"1.1\" viewBox=\"0 0 %d %d\">\n", m.W, m.H)
	if m.Border > 0 {
		fmt.Fprintf(&b, "<style>polygon{stroke:#000;stroke-width:%d;vector-effect:non-scaling-stroke}</style>\n", m.Border)
	}
	for i, t := range m.tris {
		r, g, bl := m.rgb(i)
		fmt.Fprintf(&b, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f\" fill=\"#%02x%02x%02x\" />\n",
			m.pts[t[0]].x+0.5, m.pts[t[0]].y+0.5, m.pts[t[1]].x+0.5, m.pts[t[1]].y+0.5, m.pts[t[2]].x+0.5, m.pts[t[2]].y+0.5, r, g, bl)
	}
	b.WriteString("</svg>")
	return b.String()
}

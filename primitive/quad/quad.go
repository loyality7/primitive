// Package quad builds an image from a quadtree: the picture starts as one
// rectangle filled with its average color, and the rectangle with the largest
// error is split into four again and again.
//
// Cells are drawn as outlined squares (the default), as circles on black, or as
// two triangles each, with the diagonal that fits the picture best.
//
// The idea comes from Michael Fogleman's Quads. This is a separate
// implementation written from the algorithm description and from looking at
// the pictures it produces.
package quad

import (
	"container/heap"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/imageio"
)

// rect is the half-open pixel rectangle [x0,x1) x [y0,y1) of the input image.
type rect struct{ x0, y0, x1, y1 int }

// children splits r in half on each axis it can. A 1 pixel wide side is not
// split, so a rectangle gives 1, 2 or 4 children.
func (r rect) children() []rect {
	xs := []int{r.x0, r.x1}
	if r.x1-r.x0 >= 2 {
		xs = []int{r.x0, r.x0 + (r.x1-r.x0)/2, r.x1}
	}
	ys := []int{r.y0, r.y1}
	if r.y1-r.y0 >= 2 {
		ys = []int{r.y0, r.y0 + (r.y1-r.y0)/2, r.y1}
	}
	var out []rect
	for i := 0; i+1 < len(ys); i++ {
		for j := 0; j+1 < len(xs); j++ {
			out = append(out, rect{xs[j], ys[i], xs[j+1], ys[i+1]})
		}
	}
	return out
}

type node struct {
	rect
	r, g, b uint8
	err     float64 // sum of squared errors of the fill against the input
	id      int     // index in Model.created

	// Triangle cells: the cell is cut along one diagonal into two triangles.
	// r,g,b is the first one (upper right for diagonal 0, upper left for
	// diagonal 1) and r2,g2,b2 the other.
	tri        bool
	diag       uint8
	r2, g2, b2 uint8
}

// record is a node with the split numbers that created and removed it.
type record struct {
	node
	born  int // the split that created it, 0 for the first rectangle
	split int // the split that replaced it, 0 while it is still a leaf
}

// leafHeap is a max-heap on error: the worst rectangle is always on top.
type leafHeap []node

func (h leafHeap) Len() int            { return len(h) }
func (h leafHeap) Less(i, j int) bool  { return h[i].err > h[j].err }
func (h leafHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *leafHeap) Push(x interface{}) { *h = append(*h, x.(node)) }
func (h *leafHeap) Pop() interface{} {
	old := *h
	n := old[len(old)-1]
	*h = old[:len(old)-1]
	return n
}

// Model is a quadtree approximation of one input image.
type Model struct {
	W, H   int // input size
	Sw, Sh int // output size

	// Border is the width in output pixels of the dark outline around every
	// square. 0 gives plain squares. It does not apply to circles.
	Border int
	// Circles draws each cell as a circle on a black background.
	Circles bool
	// Triangles draws each cell as two triangles. Set it, then call Restart,
	// before splitting. It cannot be combined with Circles.
	Triangles bool
	// Palette, when set, snaps every drawn color to its nearest entry. It
	// changes how cells are drawn, not which cells are split.
	Palette []colors.Color
	// AnimStep makes SVG() fade each split in this many seconds after the
	// previous one. 0 gives the plain, static picture.
	AnimStep float64
	// NoHistory skips recording the splits. Live video needs it for speed;
	// the animated SVG and Frames need the history, so do not use them then.
	NoHistory bool

	pix     []uint8   // the frame being analysed, RGB
	avg     []float32 // running average of earlier frames, for smoothing
	leaves  leafHeap
	created []record // every rectangle ever created, for the animated SVG
	order   []rect   // the rectangle split at each step, to replay frames
	total   float64  // sum of the errors of all leaves
	stride  int
	ii      [4][]int64 // integral images: R, G, B sums and sum of squares of all channels
}

// NewModel prepares a model for img and fits one rectangle to the whole
// image. size is the longer side of the output. Squares get a 1 pixel outline.
func NewModel(img image.Image, size int) *Model {
	rgba := imageio.ImageToRGBA(img)
	b := rgba.Bounds()
	m := NewEmpty(b.Dx(), b.Dy(), size)
	if err := m.SetFrame(rgba, 0); err != nil {
		panic(err) // cannot happen: the size comes from the image itself
	}
	return m
}

// NewEmpty makes a model for w x h pictures with nothing in it yet. Call
// SetFrame before using it. It is meant for live video, where one model is
// reused for every frame.
func NewEmpty(w, h, size int) *Model {
	m := &Model{W: w, H: h, Border: 1}
	aspect := float64(w) / float64(h)
	if aspect >= 1 {
		m.Sw, m.Sh = size, int(float64(size)/aspect)
	} else {
		m.Sw, m.Sh = int(float64(size)*aspect), size
	}
	m.Sw, m.Sh = max(m.Sw, 1), max(m.Sh, 1)
	m.stride = w + 1
	for i := range m.ii {
		m.ii[i] = make([]int64, m.stride*(h+1))
	}
	m.pix = make([]uint8, w*h*3)
	return m
}

// SetFrame loads a picture of the model's size and restarts the tree from one
// square. smooth (0 to 1) blends the frame into a running average of the
// earlier ones, which calms the flicker of live video: 0 uses the frame as it is.
func (m *Model) SetFrame(img *image.RGBA, smooth float64) error {
	b := img.Bounds()
	if b.Dx() != m.W || b.Dy() != m.H {
		return fmt.Errorf("frame is %dx%d, the model is %dx%d", b.Dx(), b.Dy(), m.W, m.H)
	}
	smooth = math.Max(0, math.Min(smooth, 0.98))
	fresh := false
	if smooth == 0 {
		m.avg = nil
	} else if m.avg == nil {
		m.avg, fresh = make([]float32, m.W*m.H*3), true
	}
	s32 := float32(smooth)
	k := 0
	for y := 0; y < m.H; y++ {
		o := img.PixOffset(b.Min.X, b.Min.Y+y)
		for x := 0; x < m.W; x++ {
			for c := 0; c < 3; c++ {
				v := img.Pix[o+c]
				if smooth > 0 {
					if fresh {
						m.avg[k] = float32(v)
					} else {
						m.avg[k] = m.avg[k]*s32 + float32(v)*(1-s32)
					}
					v = uint8(m.avg[k] + 0.5)
				}
				m.pix[k] = v
				k++
			}
			o += 4
		}
	}
	for y := 0; y < m.H; y++ {
		var row [4]int64
		for x := 0; x < m.W; x++ {
			o := (y*m.W + x) * 3
			r, g, bl := int64(m.pix[o]), int64(m.pix[o+1]), int64(m.pix[o+2])
			row[0] += r
			row[1] += g
			row[2] += bl
			row[3] += r*r + g*g + bl*bl
			for c := range m.ii {
				m.ii[c][(y+1)*m.stride+x+1] = m.ii[c][y*m.stride+x+1] + row[c]
			}
		}
	}
	m.Restart()
	return nil
}

// Restart starts the tree again from one cell, using the frame already loaded.
// Call it after changing Triangles so the first cell is fitted the new way.
func (m *Model) Restart() {
	root := m.measure(rect{0, 0, m.W, m.H})
	m.leaves = append(m.leaves[:0], root)
	m.order = m.order[:0]
	m.created = m.created[:0]
	if !m.NoHistory {
		m.created = append(m.created, record{node: root})
	}
	m.total = root.err
}

// sum returns the sum over r of integral image c.
func (m *Model) sum(c int, r rect) int64 {
	ii := m.ii[c]
	return ii[r.y1*m.stride+r.x1] - ii[r.y0*m.stride+r.x1] - ii[r.y1*m.stride+r.x0] + ii[r.y0*m.stride+r.x0]
}

// measure fits the cell r and computes its error.
func (m *Model) measure(r rect) node {
	if m.Triangles && r.x1-r.x0 >= 2 && r.y1-r.y0 >= 2 {
		return m.measureTriangles(r)
	}
	return m.measureFlat(r)
}

// spanSum is the sum of channel c over pixels [xa,xb) of row y.
func (m *Model) spanSum(c, y, xa, xb int) int64 {
	ii := m.ii[c]
	return ii[(y+1)*m.stride+xb] - ii[y*m.stride+xb] - ii[(y+1)*m.stride+xa] + ii[y*m.stride+xa]
}

// triCut says where row dy of a w x h cell is cut by a diagonal. Diagonal 0
// runs from the top left to the bottom right corner: the first triangle is the
// part of the row from the cut to the right edge. Diagonal 1 runs from the top
// right to the bottom left corner: the first triangle is the part from the left
// edge to the cut. A pixel counts for the side its center is on, so the two
// triangles always tile the cell.
func triCut(diag, dy, w, h int) int {
	var cut int
	if diag == 0 {
		if n := (2*dy+1)*w - h; n >= 0 {
			cut = n/(2*h) + 1
		}
	} else if n := 2*w*h - (2*dy+1)*w - h; n > 0 {
		cut = (n + 2*h - 1) / (2 * h)
	}
	return max(0, min(cut, w))
}

// measureTriangles fits two triangles to r, trying both diagonals.
func (m *Model) measureTriangles(r rect) node {
	w, h := r.x1-r.x0, r.y1-r.y0
	best := node{err: math.Inf(1)}
	for diag := 0; diag < 2; diag++ {
		var n [2]int64
		var s [2][4]int64
		for dy := 0; dy < h; dy++ {
			cut := triCut(diag, dy, w, h)
			spans := [2][2]int{{cut, w}, {0, cut}} // first triangle, second triangle
			if diag == 1 {
				spans = [2][2]int{{0, cut}, {cut, w}}
			}
			for t, sp := range spans {
				if sp[1] <= sp[0] {
					continue
				}
				n[t] += int64(sp[1] - sp[0])
				for c := 0; c < 4; c++ {
					s[t][c] += m.spanSum(c, r.y0+dy, r.x0+sp[0], r.x0+sp[1])
				}
			}
		}
		var errSum float64
		var col [2][3]uint8
		for t := 0; t < 2; t++ {
			if n[t] == 0 {
				continue
			}
			sq := float64(s[t][3])
			for c := 0; c < 3; c++ {
				col[t][c] = uint8((s[t][c] + n[t]/2) / n[t])
				sq -= float64(s[t][c]) * float64(s[t][c]) / float64(n[t])
			}
			errSum += math.Max(sq, 0)
		}
		if n[0] == 0 {
			col[0] = col[1]
		}
		if n[1] == 0 {
			col[1] = col[0]
		}
		if errSum < best.err {
			best = node{rect: r, tri: true, diag: uint8(diag), err: errSum,
				r: col[0][0], g: col[0][1], b: col[0][2], r2: col[1][0], g2: col[1][1], b2: col[1][2]}
		}
	}
	return best
}

// measureFlat fits the average color to r and computes its error.
func (m *Model) measureFlat(r rect) node {
	n := int64((r.x1 - r.x0) * (r.y1 - r.y0))
	sq := float64(m.sum(3, r))
	var mean [3]uint8
	for c := 0; c < 3; c++ {
		s := m.sum(c, r)
		mean[c] = uint8((s + n/2) / n)
		sq -= float64(s) * float64(s) / float64(n)
	}
	if sq < 0 { // rounding
		sq = 0
	}
	return node{rect: r, r: mean[0], g: mean[1], b: mean[2], err: sq}
}

// Split splits the rectangle with the largest error. It reports false when
// there is nothing left to improve.
func (m *Model) Split() bool {
	if len(m.leaves) == 0 || m.leaves[0].err <= 0 {
		return false
	}
	worst := heap.Pop(&m.leaves).(node)
	m.total -= worst.err
	step := len(m.order) + 1
	if !m.NoHistory {
		m.created[worst.id].split = step
	}
	for _, c := range worst.children() {
		n := m.measure(c)
		if !m.NoHistory {
			n.id = len(m.created)
			m.created = append(m.created, record{node: n, born: step})
		}
		m.total += n.err
		heap.Push(&m.leaves, n)
	}
	if !m.NoHistory {
		m.order = append(m.order, worst.rect)
	}
	return true
}

// Leaves is the number of cells drawn now.
func (m *Model) Leaves() int { return len(m.leaves) }

// Score is the root mean square error of the fill colors against the input,
// from 0 (identical) to 1, on the same scale the shape engine reports.
func (m *Model) Score() float64 {
	if m.total < 0 {
		return 0
	}
	return math.Sqrt(m.total/float64(m.W*m.H*3)) / 255
}

func fillRect(im *image.RGBA, x0, y0, x1, y1 int, r, g, b uint8) {
	for y := y0; y < y1; y++ {
		o := im.PixOffset(x0, y)
		for x := x0; x < x1; x++ {
			im.Pix[o], im.Pix[o+1], im.Pix[o+2], im.Pix[o+3] = r, g, b, 255
			o += 4
		}
	}
}

// disc draws an anti-aliased circle inscribed in the cell onto a black image.
func disc(im *image.RGBA, n node, x0, y0, x1, y1 int) {
	cx, cy := float64(x0+x1)/2, float64(y0+y1)/2
	rad := float64(min(x1-x0, y1-y0)) / 2
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			a := rad - math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) + 0.5
			if a <= 0 {
				continue
			}
			a = math.Min(a, 1)
			o := im.PixOffset(x, y)
			im.Pix[o] = uint8(float64(n.r)*a + 0.5)
			im.Pix[o+1] = uint8(float64(n.g)*a + 0.5)
			im.Pix[o+2] = uint8(float64(n.b)*a + 0.5)
		}
	}
}

// snap moves the colors of a cell to the palette, if there is one.
func (m *Model) snap(n node) node {
	if len(m.Palette) == 0 {
		return n
	}
	c := colors.Snap(colors.Color{R: int(n.r), G: int(n.g), B: int(n.b), A: 255}, m.Palette)
	n.r, n.g, n.b = uint8(c.R), uint8(c.G), uint8(c.B)
	if n.tri {
		c = colors.Snap(colors.Color{R: int(n.r2), G: int(n.g2), B: int(n.b2), A: 255}, m.Palette)
		n.r2, n.g2, n.b2 = uint8(c.R), uint8(c.G), uint8(c.B)
	}
	return n
}

// triCell paints a cell as two triangles. The diagonal is anti-aliased by
// blending the two colors across it, and drawn dark when border > 0.
func triCell(im *image.RGBA, n node, x0, y0, x1, y1, border int) {
	w, h := float64(x1-x0), float64(y1-y0)
	if x1-x0 < 2 || y1-y0 < 2 { // too small to cut
		fillRect(im, x0, y0, x1, y1, uint8((int(n.r)+int(n.r2))/2), uint8((int(n.g)+int(n.g2))/2), uint8((int(n.b)+int(n.b2))/2))
		return
	}
	length := math.Hypot(w, h)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			px, py := float64(x-x0)+0.5, float64(y-y0)+0.5
			var d float64 // distance to the diagonal, positive on the first triangle's side
			if n.diag == 0 {
				d = (px*h - py*w) / length
			} else {
				d = (w*h - px*h - py*w) / length
			}
			a := math.Max(0, math.Min(1, 0.5+d))
			line := 0.0
			if border > 0 {
				line = math.Max(0, math.Min(1, float64(border)/2+0.5-math.Abs(d)))
			}
			mix := func(first, second uint8) uint8 {
				v := (float64(first)*a + float64(second)*(1-a)) * (1 - line)
				return uint8(v + 0.5)
			}
			o := im.PixOffset(x, y)
			im.Pix[o], im.Pix[o+1], im.Pix[o+2], im.Pix[o+3] = mix(n.r, n.r2), mix(n.g, n.g2), mix(n.b, n.b2), 255
		}
	}
}

// paint draws the leaves onto an output sized image. Output pixel edges are
// computed by integer scaling, so neighbouring cells never leave gaps.
func (m *Model) paint(leaves []node) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
	m.paintInto(out, leaves)
	return out
}

// RenderInto draws the current state into dst, which must be Sw x Sh. Using
// the same dst for every video frame avoids allocating one each time.
func (m *Model) RenderInto(dst *image.RGBA) { m.paintInto(dst, m.leaves) }

func (m *Model) paintInto(out *image.RGBA, leaves []node) {
	if m.Circles {
		fillRect(out, 0, 0, m.Sw, m.Sh, 0, 0, 0)
	}
	for _, n := range leaves {
		n = m.snap(n)
		x0, x1 := n.x0*m.Sw/m.W, n.x1*m.Sw/m.W
		y0, y1 := n.y0*m.Sh/m.H, n.y1*m.Sh/m.H
		if m.Circles {
			disc(out, n, x0, y0, x1, y1)
			continue
		}
		if n.tri {
			triCell(out, n, x0, y0, x1, y1, m.Border)
		} else {
			fillRect(out, x0, y0, x1, y1, n.r, n.g, n.b)
		}
		if b := m.Border; b > 0 {
			// left and top edge of every cell, plus the right and bottom edge
			// of the picture: one line of width b between any two cells
			fillRect(out, x0, y0, min(x0+b, x1), y1, 0, 0, 0)
			fillRect(out, x0, y0, x1, min(y0+b, y1), 0, 0, 0)
			if x1 == m.Sw {
				fillRect(out, max(x1-b, x0), y0, x1, y1, 0, 0, 0)
			}
			if y1 == m.Sh {
				fillRect(out, x0, max(y1-b, y0), x1, y1, 0, 0, 0)
			}
		}
	}
}

// Image renders the current state.
func (m *Model) Image() image.Image { return m.paint(m.leaves) }

// SVG renders the current state, one element per leaf. With AnimStep set it
// draws every cell in the order it was created instead, each fading in after
// its split, so the picture builds up and ends identical to the static one.
// Squares are simply covered by their children; circles leave gaps, so a
// circle fades out when its children appear.
func (m *Model) SVG() string {
	var b strings.Builder
	crisp := ""
	if !m.Circles {
		crisp = ` shape-rendering="crispEdges"`
	}
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" version=\"1.1\" viewBox=\"0 0 %d %d\"%s>\n", m.W, m.H, crisp)
	css := ""
	if !m.Circles && m.Border > 0 {
		sel := "rect"
		if m.Triangles {
			sel = "rect,polygon"
		}
		css += fmt.Sprintf("%s{stroke:#000;stroke-width:%d;vector-effect:non-scaling-stroke}", sel, m.Border)
	}
	if m.AnimStep > 0 {
		css += ".a{opacity:0;animation:p .25s ease-out forwards}" +
			".b{opacity:0;animation:p .25s ease-out forwards,q .01s linear forwards}" +
			".c{animation:q .01s linear forwards}" +
			"@keyframes p{to{opacity:1}}@keyframes q{to{opacity:0}}"
	}
	if css != "" {
		fmt.Fprintf(&b, "<style>%s</style>\n", css)
	}
	if m.Circles {
		fmt.Fprintf(&b, "<rect width=\"%d\" height=\"%d\" fill=\"#000\" />\n", m.W, m.H)
	}
	cell := func(n node, extra string) {
		n = m.snap(n)
		if n.tri {
			x0, y0, x1, y1 := n.x0, n.y0, n.x1, n.y1
			first, second := [6]int{x0, y0, x1, y0, x1, y1}, [6]int{x0, y0, x1, y1, x0, y1} // diagonal 0
			if n.diag == 1 {
				first, second = [6]int{x0, y0, x1, y0, x0, y1}, [6]int{x1, y0, x1, y1, x0, y1}
			}
			for i, pts := range [2][6]int{first, second} {
				r, g, bl := n.r, n.g, n.b
				if i == 1 {
					r, g, bl = n.r2, n.g2, n.b2
				}
				fmt.Fprintf(&b, "<polygon points=\"%d,%d %d,%d %d,%d\" fill=\"#%02x%02x%02x\"%s />\n",
					pts[0], pts[1], pts[2], pts[3], pts[4], pts[5], r, g, bl, extra)
			}
			return
		}
		if m.Circles {
			fmt.Fprintf(&b, "<circle cx=\"%g\" cy=\"%g\" r=\"%g\" fill=\"#%02x%02x%02x\"%s />\n",
				float64(n.x0+n.x1)/2, float64(n.y0+n.y1)/2, float64(min(n.x1-n.x0, n.y1-n.y0))/2, n.r, n.g, n.b, extra)
			return
		}
		fmt.Fprintf(&b, "<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" fill=\"#%02x%02x%02x\"%s />\n",
			n.x0, n.y0, n.x1-n.x0, n.y1-n.y0, n.r, n.g, n.b, extra)
	}
	if m.AnimStep == 0 {
		for _, n := range m.leaves {
			cell(n, "")
		}
		b.WriteString("</svg>")
		return b.String()
	}
	sec := func(step int) float64 { return float64(step) * m.AnimStep }
	for i, rec := range m.created {
		extra := ""
		switch {
		case i == 0 && !m.Circles:
			// the first square is the background
		case i == 0 && rec.split > 0:
			extra = fmt.Sprintf(" class=\"c\" style=\"animation-delay:%.3fs\"", sec(rec.split))
		case i == 0:
		case m.Circles && rec.split > 0:
			extra = fmt.Sprintf(" class=\"b\" style=\"animation-delay:%.3fs,%.3fs\"", sec(rec.born), sec(rec.split))
		default:
			extra = fmt.Sprintf(" class=\"a\" style=\"animation-delay:%.3fs\"", sec(rec.born))
		}
		cell(rec.node, extra)
	}
	b.WriteString("</svg>")
	return b.String()
}

// maxFrames limits the frames of an animation.
const maxFrames = 60

// Frames replays the splits and returns up to maxFrames snapshots from the
// single rectangle to the current state. The argument is unused and only
// there so Model and the shape model share one interface.
func (m *Model) Frames(_ float64) []image.Image {
	steps := len(m.order)
	want := map[int]bool{}
	for i := 0; i < maxFrames; i++ {
		want[i*steps/(maxFrames-1)] = true
	}
	cur := map[rect]node{}
	root := m.measure(rect{0, 0, m.W, m.H})
	cur[root.rect] = root
	var frames []image.Image
	snapshot := func() {
		leaves := make([]node, 0, len(cur))
		for _, n := range cur {
			leaves = append(leaves, n)
		}
		frames = append(frames, m.paint(leaves))
	}
	if want[0] {
		snapshot()
	}
	for i, parent := range m.order {
		delete(cur, parent)
		for _, c := range parent.children() {
			cur[c] = m.measure(c)
		}
		if want[i+1] {
			snapshot()
		}
	}
	return frames
}

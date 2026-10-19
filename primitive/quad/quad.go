// Package quad builds an image from a quadtree: the picture starts as one
// rectangle filled with its average color, and the rectangle with the largest
// error is split into four again and again.
//
// Cells are drawn as outlined squares (the default) or as circles on black.
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
	// AnimStep makes SVG() fade each split in this many seconds after the
	// previous one. 0 gives the plain, static picture.
	AnimStep float64

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
	m := &Model{W: b.Dx(), H: b.Dy(), Border: 1}
	aspect := float64(m.W) / float64(m.H)
	if aspect >= 1 {
		m.Sw, m.Sh = size, int(float64(size)/aspect)
	} else {
		m.Sw, m.Sh = int(float64(size)*aspect), size
	}
	m.Sw, m.Sh = max(m.Sw, 1), max(m.Sh, 1)
	m.stride = m.W + 1
	for i := range m.ii {
		m.ii[i] = make([]int64, m.stride*(m.H+1))
	}
	for y := 0; y < m.H; y++ {
		var row [4]int64
		for x := 0; x < m.W; x++ {
			o := rgba.PixOffset(b.Min.X+x, b.Min.Y+y)
			r, g, bl := int64(rgba.Pix[o]), int64(rgba.Pix[o+1]), int64(rgba.Pix[o+2])
			row[0] += r
			row[1] += g
			row[2] += bl
			row[3] += r*r + g*g + bl*bl
			for c := range m.ii {
				m.ii[c][(y+1)*m.stride+x+1] = m.ii[c][y*m.stride+x+1] + row[c]
			}
		}
	}
	root := m.measure(rect{0, 0, m.W, m.H})
	m.leaves = leafHeap{root}
	m.created = []record{{node: root}}
	m.total = root.err
	return m
}

// sum returns the sum over r of integral image c.
func (m *Model) sum(c int, r rect) int64 {
	ii := m.ii[c]
	return ii[r.y1*m.stride+r.x1] - ii[r.y0*m.stride+r.x1] - ii[r.y1*m.stride+r.x0] + ii[r.y0*m.stride+r.x0]
}

// measure fits the average color to r and computes its error.
func (m *Model) measure(r rect) node {
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
	m.created[worst.id].split = step
	for _, c := range worst.children() {
		n := m.measure(c)
		n.id = len(m.created)
		m.created = append(m.created, record{node: n, born: step})
		m.total += n.err
		heap.Push(&m.leaves, n)
	}
	m.order = append(m.order, worst.rect)
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

// paint draws the leaves onto an output sized image. Output pixel edges are
// computed by integer scaling, so neighbouring cells never leave gaps.
func (m *Model) paint(leaves []node) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
	if m.Circles {
		fillRect(out, 0, 0, m.Sw, m.Sh, 0, 0, 0)
	}
	for _, n := range leaves {
		x0, x1 := n.x0*m.Sw/m.W, n.x1*m.Sw/m.W
		y0, y1 := n.y0*m.Sh/m.H, n.y1*m.Sh/m.H
		if m.Circles {
			disc(out, n, x0, y0, x1, y1)
			continue
		}
		fillRect(out, x0, y0, x1, y1, n.r, n.g, n.b)
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
	return out
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
		css += fmt.Sprintf("rect{stroke:#000;stroke-width:%d;vector-effect:non-scaling-stroke}", m.Border)
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

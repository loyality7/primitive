// Package quad builds an image from a quadtree: the picture starts as one
// rectangle filled with its average color, and the rectangle with the largest
// error is split into four again and again.
//
// The idea comes from Michael Fogleman's Quads. This is a separate
// implementation written from the algorithm description.
package quad

import (
	"container/heap"
	"fmt"
	"image"
	"image/color"
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

	leaves leafHeap
	order  []rect  // the rectangle split at each step, to replay frames
	total  float64 // sum of the errors of all leaves
	stride int
	ii     [4][]int64 // integral images: R, G, B sums and sum of squares of all channels
}

// NewModel prepares a model for img and fits one rectangle to the whole
// image. size is the longer side of the output.
func NewModel(img image.Image, size int) *Model {
	rgba := imageio.ImageToRGBA(img)
	b := rgba.Bounds()
	m := &Model{W: b.Dx(), H: b.Dy()}
	aspect := float64(m.W) / float64(m.H)
	if aspect >= 1 {
		m.Sw, m.Sh = size, int(float64(size)/aspect)
	} else {
		m.Sw, m.Sh = int(float64(size)*aspect), size
	}
	if m.Sh < 1 {
		m.Sh = 1
	}
	if m.Sw < 1 {
		m.Sw = 1
	}
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
	var sums [3]int64
	var sq float64 = float64(m.sum(3, r))
	var mean [3]uint8
	for c := 0; c < 3; c++ {
		sums[c] = m.sum(c, r)
		mean[c] = uint8((sums[c] + n/2) / n)
		sq -= float64(sums[c]) * float64(sums[c]) / float64(n)
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
	for _, c := range worst.children() {
		n := m.measure(c)
		m.total += n.err
		heap.Push(&m.leaves, n)
	}
	m.order = append(m.order, worst.rect)
	return true
}

// Leaves is the number of rectangles drawn now.
func (m *Model) Leaves() int { return len(m.leaves) }

// Score is the root mean square error against the input, from 0 (identical)
// to 1, on the same scale the shape engine reports.
func (m *Model) Score() float64 {
	if m.total < 0 {
		return 0
	}
	return math.Sqrt(m.total/float64(m.W*m.H*3)) / 255
}

// paint draws the leaves onto an output sized image. Output pixel edges are
// computed by integer scaling, so neighbouring rectangles never leave gaps.
func (m *Model) paint(leaves []node) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
	for _, n := range leaves {
		x0, x1 := n.x0*m.Sw/m.W, n.x1*m.Sw/m.W
		y0, y1 := n.y0*m.Sh/m.H, n.y1*m.Sh/m.H
		c := color.RGBA{n.r, n.g, n.b, 255}
		for y := y0; y < y1; y++ {
			o := out.PixOffset(x0, y)
			for x := x0; x < x1; x++ {
				out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = c.R, c.G, c.B, 255
				o += 4
			}
		}
	}
	return out
}

// Image renders the current state.
func (m *Model) Image() image.Image { return m.paint(m.leaves) }

// SVG renders the current state as one rect per leaf.
func (m *Model) SVG() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" version=\"1.1\" viewBox=\"0 0 %d %d\" shape-rendering=\"crispEdges\">\n", m.W, m.H)
	for _, n := range m.leaves {
		fmt.Fprintf(&b, "<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" fill=\"#%02x%02x%02x\" />\n",
			n.x0, n.y0, n.x1-n.x0, n.y1-n.y0, n.r, n.g, n.b)
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

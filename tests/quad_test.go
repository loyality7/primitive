package tests

import (
	"image"
	"image/color"
	"math/rand"
	"strings"
	"testing"

	"github.com/loyality7/primitive/primitive/quad"
)

func noisyImage(w, h int, seed int64) *image.RGBA {
	rnd := rand.New(rand.NewSource(seed))
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetRGBA(x, y, color.RGBA{R: uint8(rnd.Intn(256)), G: uint8(rnd.Intn(256)), B: uint8(rnd.Intn(256)), A: 255})
		}
	}
	return im
}

func TestQuadErrorShrinks(t *testing.T) {
	m := quad.NewModel(gradientImage(), 48)
	prev := m.Score()
	if prev <= 0 {
		t.Fatal("one flat rectangle must have an error on a gradient")
	}
	for i := 0; i < 200; i++ {
		if !m.Split() {
			t.Fatalf("ran out of splits after %d", i)
		}
		if s := m.Score(); s > prev+1e-9 {
			t.Fatalf("score went up at split %d: %v -> %v", i, prev, s)
		}
		prev = m.Score()
	}
	if m.Score() > 0.02 {
		t.Fatalf("200 splits should fit a smooth gradient well, got %v", m.Score())
	}
}

func TestQuadLeafCount(t *testing.T) {
	m := quad.NewModel(noisyImage(64, 64, 1), 64)
	for i := 0; i < 50; i++ {
		m.Split()
	}
	if got := m.Leaves(); got != 1+3*50 {
		t.Fatalf("leaves = %d, want %d", got, 1+3*50)
	}
}

func TestQuadExactWhenFullySplit(t *testing.T) {
	src := noisyImage(8, 8, 2)
	m := quad.NewModel(src, 8)
	m.Border = 0 // outlines would cover pixels
	for m.Split() {
	}
	if m.Score() != 0 {
		t.Fatalf("fully split score = %v, want 0", m.Score())
	}
	out := m.Image().(*image.RGBA)
	for i := range src.Pix {
		if out.Pix[i] != src.Pix[i] {
			t.Fatalf("pixel byte %d differs: got %d want %d", i, out.Pix[i], src.Pix[i])
		}
	}
}

func TestQuadTilesNonSquareImage(t *testing.T) {
	m := quad.NewModel(noisyImage(40, 24, 3), 100)
	for i := 0; i < 30; i++ {
		m.Split()
	}
	out := m.Image().(*image.RGBA)
	if b := out.Bounds(); b.Dx() != 100 || b.Dy() != 60 {
		t.Fatalf("output size %v, want 100x60", b)
	}
	for i := 3; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 255 {
			t.Fatalf("gap in the output at byte %d", i)
		}
	}
}

func TestQuadFlatImageStops(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range im.Pix {
		im.Pix[i] = 200
	}
	m := quad.NewModel(im, 32)
	if m.Split() {
		t.Fatal("a flat image has nothing to split")
	}
	if m.Leaves() != 1 || m.Score() != 0 {
		t.Fatalf("leaves=%d score=%v", m.Leaves(), m.Score())
	}
}

func TestQuadSVG(t *testing.T) {
	m := quad.NewModel(noisyImage(16, 16, 4), 16)
	for i := 0; i < 5; i++ {
		m.Split()
	}
	svg := m.SVG()
	if got := strings.Count(svg, "<rect"); got != m.Leaves() {
		t.Fatalf("%d rects for %d leaves", got, m.Leaves())
	}
	if !strings.Contains(svg, `viewBox="0 0 16 16"`) {
		t.Fatal("viewBox missing")
	}
}

func TestQuadFrames(t *testing.T) {
	m := quad.NewModel(noisyImage(32, 32, 5), 32)
	m.Border = 0 // the first frame is only flat without outlines
	for i := 0; i < 100; i++ {
		m.Split()
	}
	frames := m.Frames(0)
	if len(frames) < 2 || len(frames) > 60 {
		t.Fatalf("%d frames, want between 2 and 60", len(frames))
	}
	first, last := frames[0].(*image.RGBA), frames[len(frames)-1].(*image.RGBA)
	for i := 4; i < len(first.Pix); i += 4 {
		if first.Pix[i] != first.Pix[0] || first.Pix[i+1] != first.Pix[1] {
			t.Fatal("first frame should be one flat color")
		}
	}
	final := m.Image().(*image.RGBA)
	for i := range final.Pix {
		if last.Pix[i] != final.Pix[i] {
			t.Fatal("last frame must equal the current image")
		}
	}
}

func TestQuadAnimatedSVG(t *testing.T) {
	m := quad.NewModel(noisyImage(32, 32, 6), 32)
	m.Border = 0 // the outline adds its own style element
	for i := 0; i < 5; i++ {
		m.Split()
	}
	static := m.SVG()
	if strings.Contains(static, "animation") || strings.Contains(static, "<style>") {
		t.Fatal("static SVG must not contain animation markup")
	}
	m.AnimStep = 0.1
	anim := m.SVG()
	// the first square plus 4 children per split
	if got := strings.Count(anim, "<rect"); got != 1+4*5 {
		t.Fatalf("animated SVG has %d rects, want %d", got, 1+4*5)
	}
	if !strings.Contains(anim, "animation-delay:0.500s") {
		t.Fatal("the last split should start at 5 x 0.1 s")
	}
	if strings.Contains(strings.SplitN(anim, "<rect", 2)[1][:80], "class=") {
		t.Fatal("the background square must not be animated")
	}
	// every square of the final picture is drawn, so the animation ends on the same image
	for _, line := range strings.Split(static, "\n") {
		if !strings.HasPrefix(line, "<rect") {
			continue
		}
		prefix := strings.TrimSuffix(line, " />")
		if !strings.Contains(anim, prefix) {
			t.Fatalf("final square missing from the animation: %s", line)
		}
	}
}

func TestQuadOutlines(t *testing.T) {
	src := noisyImage(64, 64, 7)
	plain := quad.NewModel(src, 64)
	plain.Border = 0
	lined := quad.NewModel(src, 64) // outlines are the default
	for _, m := range []*quad.Model{plain, lined} {
		for i := 0; i < 20; i++ {
			m.Split()
		}
	}
	a, b := plain.Image().(*image.RGBA), lined.Image().(*image.RGBA)
	black := func(im *image.RGBA, x, y int) bool {
		o := im.PixOffset(x, y)
		return im.Pix[o] == 0 && im.Pix[o+1] == 0 && im.Pix[o+2] == 0
	}
	// the border of the picture is always outlined, and 20 splits of noise leave 32 pixel cells
	for i := 0; i < 64; i++ {
		if !black(b, i, 0) || !black(b, 0, i) || !black(b, i, 63) || !black(b, 63, i) {
			t.Fatalf("picture border is not outlined at %d", i)
		}
	}
	if black(a, 10, 0) && black(a, 0, 10) && black(a, 10, 63) {
		t.Fatal("plain squares should not be outlined")
	}
	// the line between the first four cells
	if !black(b, 32, 10) || !black(b, 10, 32) {
		t.Fatal("no outline between neighbouring cells")
	}
	// inside a cell the colors are the same with and without outlines
	if a.Pix[a.PixOffset(20, 20)] != b.Pix[b.PixOffset(20, 20)] {
		t.Fatal("outlines must not change the fill")
	}
	if !strings.Contains(lined.SVG(), "stroke-width:1") || strings.Contains(plain.SVG(), "stroke") {
		t.Fatal("SVG outline style should follow the Border setting")
	}
}

func TestQuadCircles(t *testing.T) {
	m := quad.NewModel(noisyImage(64, 64, 8), 64)
	m.Circles = true
	for i := 0; i < 10; i++ {
		m.Split()
	}
	im := m.Image().(*image.RGBA)
	// a circle inscribed in a square covers pi/4 = 78 percent of it, the rest is black
	lit := 0
	for i := 0; i < len(im.Pix); i += 4 {
		if im.Pix[i]|im.Pix[i+1]|im.Pix[i+2] != 0 {
			lit++
		}
	}
	frac := float64(lit) / float64(len(im.Pix)/4)
	if frac < 0.6 || frac > 0.9 {
		t.Fatalf("%.2f of the picture is colored, want about 0.78", frac)
	}
	if o := im.PixOffset(0, 0); im.Pix[o]|im.Pix[o+1]|im.Pix[o+2] != 0 {
		t.Fatal("the corner of the picture should be black background")
	}
	svg := m.SVG()
	if got := strings.Count(svg, "<circle"); got != m.Leaves() {
		t.Fatalf("%d circles for %d leaves", got, m.Leaves())
	}
	if !strings.Contains(svg, `fill="#000"`) || strings.Contains(svg, "crispEdges") {
		t.Fatal("circles need a black background and anti-aliasing")
	}
}

func TestQuadCircleAnimation(t *testing.T) {
	m := quad.NewModel(noisyImage(64, 64, 9), 64)
	m.Circles = true
	for i := 0; i < 4; i++ {
		m.Split()
	}
	m.AnimStep = 0.5
	svg := m.SVG()
	if got := strings.Count(svg, "<circle"); got != 1+4*4 {
		t.Fatalf("%d circles, want %d", got, 1+4*4)
	}
	// a circle that was split fades out when its children appear
	if !strings.Contains(svg, `class="c"`) {
		t.Fatal("the first circle was split and should fade out")
	}
	if !strings.Contains(svg, `class="b"`) || !strings.Contains(svg, `class="a"`) {
		t.Fatal("expected both fade-in only and fade-in then out circles")
	}
	if got := strings.Count(svg, `class="a"`); got != m.Leaves() {
		t.Fatalf("%d circles that stay, want the %d leaves", got, m.Leaves())
	}
}

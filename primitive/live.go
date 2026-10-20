package primitive

import (
	"fmt"
	"image"
	"sort"
)

// Live video support for the shapes engine. A search for a new shape takes far
// longer than a video frame, so the shapes are kept between frames: every
// frame the target picture is replaced (SetTarget), all the shapes keep their
// place but are colored again to fit it (Recolor), the search adds a new shape
// where the picture has changed, and the weakest old shapes are dropped
// (Prune). The picture follows the camera with a short delay.

// SetTarget replaces the picture the shapes are fitted to with a new frame of
// the same size and recolors every shape for it. smooth (0 to 1) blends the
// frame into a running average of the earlier ones to calm the noise of a
// camera; 0 uses the frame as it is.
func (model *Model) SetTarget(frame *image.RGBA, smooth float64) error {
	b, t := frame.Bounds(), model.Target.Bounds()
	if b.Dx() != t.Dx() || b.Dy() != t.Dy() {
		return fmt.Errorf("frame is %dx%d, the picture is %dx%d", b.Dx(), b.Dy(), t.Dx(), t.Dy())
	}
	smooth = clampFloat(smooth, 0, 0.98)
	fresh := false
	if smooth == 0 {
		model.avg = nil
	} else if model.avg == nil {
		model.avg, fresh = make([]float32, t.Dx()*t.Dy()*3), true
	}
	s32 := float32(smooth)
	k := 0
	for y := 0; y < t.Dy(); y++ {
		src := frame.PixOffset(b.Min.X, b.Min.Y+y)
		dst := model.Target.PixOffset(t.Min.X, t.Min.Y+y)
		for x := 0; x < t.Dx(); x++ {
			for c := 0; c < 3; c++ {
				v := frame.Pix[src+c]
				if smooth > 0 {
					if fresh {
						model.avg[k] = float32(v)
					} else {
						model.avg[k] = model.avg[k]*s32 + float32(v)*(1-s32)
					}
					v = uint8(model.avg[k] + 0.5)
				}
				model.Target.Pix[dst+c] = v
				k++
			}
			model.Target.Pix[dst+3] = 255
			src += 4
			dst += 4
		}
	}
	model.Recolor()
	return nil
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Recolor draws all the shapes again, in order, giving each the color that
// fits the current target best on top of the ones before it. The shapes keep
// their place. It updates Current, Colors, Scores and Score, and remembers how
// much each shape improved the picture, for Prune.
func (model *Model) Recolor() {
	im := model.Current
	bg := model.Background.NRGBA()
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = bg.R, bg.G, bg.B, bg.A
	}
	if model.scratch == nil {
		model.scratch = image.NewRGBA(im.Bounds())
	}
	score := differenceFull(model.Target, im)
	model.gains = model.gains[:0]
	for i, sh := range model.Shapes {
		lines := sh.Rasterize()
		color := computeColor(model.Target, im, lines, model.Colors[i].A, model.Palette)
		copyLines(model.scratch, im, lines)
		drawLines(model.scratch, color, lines)
		next := differencePartial(model.Target, im, model.scratch, score, lines)
		drawLines(im, color, lines)
		model.Colors[i] = color
		model.Scores[i] = next
		model.gains = append(model.gains, score-next)
		score = next
	}
	model.Score = score
}

// Prune keeps the keep most useful shapes, judged by how much each improved
// the picture at the last Recolor, and recolors the rest. Their order is kept.
func (model *Model) Prune(keep int) {
	n := len(model.Shapes)
	if keep < 1 || n <= keep || len(model.gains) != n {
		return
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return model.gains[order[a]] > model.gains[order[b]] })
	kept := make([]bool, n)
	for _, i := range order[:keep] {
		kept[i] = true
	}
	shapes, colors, scores := model.Shapes[:0:0], model.Colors[:0:0], model.Scores[:0:0]
	for i := 0; i < n; i++ {
		if kept[i] {
			shapes = append(shapes, model.Shapes[i])
			colors = append(colors, model.Colors[i])
			scores = append(scores, model.Scores[i])
		}
	}
	model.Shapes, model.Colors, model.Scores = shapes, colors, scores
	model.Recolor()
}

// RedrawContext draws all the shapes again on the output sized canvas. Live
// video skips this for speed and shows Current instead; call it before using
// Context (to save a PNG, for example).
func (model *Model) RedrawContext() {
	model.Context = model.newContext()
	for i, sh := range model.Shapes {
		c := model.Colors[i]
		model.Context.SetRGBA255(c.R, c.G, c.B, c.A)
		sh.Draw(model.Context, model.Scale)
	}
}

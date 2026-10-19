package imageio

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math"
	"os"
	"sort"
)

// The encoder writes animated GIFs in pure Go. GIF allows 256 colors, so it
// picks one palette from all the frames with median cut and shares it
// between them, which keeps colors steady from frame to frame. Colors are
// matched with a light ordered dither, which depends only on the pixel
// position, so a region that does not change gives the same pixels in every
// frame. Each frame after the first stores only the rectangle that changed,
// with unchanged pixels transparent, and identical frames are merged.

const (
	histBits   = 5 // bits per channel when counting colors for the palette
	lookupBits = 6 // bits per channel in the nearest color lookup
	ditherAmp  = 0.3
)

// bayer is a 4x4 ordered dither matrix.
var bayer = [16]int{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

// asRGBA returns im as an RGBA image whose origin is (0, 0), copying only if needed.
func asRGBA(im image.Image) *image.RGBA {
	if r, ok := im.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := im.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Rect, im, b.Min, draw.Src)
	return dst
}

// bin sums the colors that fall into one histogram cell.
type bin struct{ n, r, g, b float64 }

func (b bin) mean() (float64, float64, float64) { return b.r / b.n, b.g / b.n, b.b / b.n }

type box struct {
	bins  []bin
	n     float64
	score float64 // how much it is worth splitting
}

func newBox(bins []bin) box {
	b := box{bins: bins}
	lo := [3]float64{255, 255, 255}
	hi := [3]float64{}
	for _, x := range bins {
		b.n += x.n
		r, g, bl := x.mean()
		for i, v := range [3]float64{r, g, bl} {
			lo[i], hi[i] = math.Min(lo[i], v), math.Max(hi[i], v)
		}
	}
	span := math.Max(hi[0]-lo[0], math.Max(hi[1]-lo[1], hi[2]-lo[2]))
	if len(bins) > 1 {
		b.score = b.n * span
	}
	return b
}

// medianCut reduces the histogram bins to at most k colors.
func medianCut(bins []bin, k int) []color.RGBA {
	boxes := []box{newBox(bins)}
	for len(boxes) < k {
		best := -1
		for i, b := range boxes {
			if b.score > 0 && (best < 0 || b.score > boxes[best].score) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		b := boxes[best]
		// split along the channel with the widest spread, at the weighted median
		lo := [3]float64{255, 255, 255}
		hi := [3]float64{}
		for _, x := range b.bins {
			r, g, bl := x.mean()
			for i, v := range [3]float64{r, g, bl} {
				lo[i], hi[i] = math.Min(lo[i], v), math.Max(hi[i], v)
			}
		}
		ch := 0
		for i := 1; i < 3; i++ {
			if hi[i]-lo[i] > hi[ch]-lo[ch] {
				ch = i
			}
		}
		key := func(x bin) float64 {
			r, g, bl := x.mean()
			return [3]float64{r, g, bl}[ch]
		}
		sort.Slice(b.bins, func(i, j int) bool { return key(b.bins[i]) < key(b.bins[j]) })
		half, acc, cut := b.n/2, 0.0, 1
		for i, x := range b.bins {
			if acc += x.n; acc >= half {
				cut = i + 1
				break
			}
		}
		if cut >= len(b.bins) {
			cut = len(b.bins) - 1
		}
		boxes[best] = newBox(b.bins[:cut])
		boxes = append(boxes, newBox(b.bins[cut:]))
	}
	out := make([]color.RGBA, len(boxes))
	for i, b := range boxes {
		var n, r, g, bl float64
		for _, x := range b.bins {
			n, r, g, bl = n+x.n, r+x.r, g+x.g, bl+x.b
		}
		out[i] = color.RGBA{uint8(r/n + 0.5), uint8(g/n + 0.5), uint8(bl/n + 0.5), 255}
	}
	return out
}

// buildPalette counts the colors of a sample of every frame (the last frame
// is sampled more densely) and reduces them to 255 colors, leaving index 0 free
// for transparency.
func buildPalette(frames []*image.RGBA) color.Palette {
	total := 0
	for _, f := range frames {
		total += len(f.Pix) / 4
	}
	stride := max(1, total/300000)
	var cells [1 << (3 * histBits)]bin
	add := func(f *image.RGBA, step int) {
		for i := 0; i < len(f.Pix); i += 4 * step {
			r, g, b := f.Pix[i], f.Pix[i+1], f.Pix[i+2]
			k := (int(r)>>(8-histBits))<<(2*histBits) | (int(g)>>(8-histBits))<<histBits | int(b)>>(8-histBits)
			c := &cells[k]
			c.n++
			c.r += float64(r)
			c.g += float64(g)
			c.b += float64(b)
		}
	}
	for _, f := range frames {
		add(f, stride)
	}
	add(frames[len(frames)-1], max(1, stride/3))
	var bins []bin
	for _, c := range cells {
		if c.n > 0 {
			bins = append(bins, c)
		}
	}
	pal := color.Palette{color.RGBA{}} // index 0: transparent
	for _, c := range medianCut(bins, 255) {
		pal = append(pal, c)
	}
	return pal
}

// quantizer maps colors to palette indexes, computing each nearest color
// the first time it is needed.
type quantizer struct {
	pal color.Palette
	lut []uint8 // 0 means not computed yet, palette index 0 is never a result
}

func newQuantizer(pal color.Palette) *quantizer {
	return &quantizer{pal: pal, lut: make([]uint8, 1<<(3*lookupBits))}
}

func (q *quantizer) index(r, g, b int) uint8 {
	const shift = 8 - lookupBits
	k := (r>>shift)<<(2*lookupBits) | (g>>shift)<<lookupBits | b>>shift
	if v := q.lut[k]; v != 0 {
		return v
	}
	cr, cg, cb := (r>>shift)<<shift+(1<<shift)/2, (g>>shift)<<shift+(1<<shift)/2, (b>>shift)<<shift+(1<<shift)/2
	best, bestD := 1, math.MaxInt
	for i := 1; i < len(q.pal); i++ {
		p := q.pal[i].(color.RGBA)
		dr, dg, db := cr-int(p.R), cg-int(p.G), cb-int(p.B)
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = i, d
		}
	}
	q.lut[k] = uint8(best)
	return uint8(best)
}

// indexes converts a frame to palette indexes with ordered dithering.
func (q *quantizer) indexes(f *image.RGBA, out []uint8) {
	w, h := f.Rect.Dx(), f.Rect.Dy()
	clamp := func(v float64) int { return int(math.Max(0, math.Min(255, v))) }
	for y := 0; y < h; y++ {
		o := f.PixOffset(0, y)
		for x := 0; x < w; x++ {
			off := (float64(bayer[(y&3)<<2|x&3]) - 7.5) * ditherAmp
			out[y*w+x] = q.index(clamp(float64(f.Pix[o])+off), clamp(float64(f.Pix[o+1])+off), clamp(float64(f.Pix[o+2])+off))
			o += 4
		}
	}
}

// EncodeGIF encodes frames as an endlessly looping animated GIF. delay is the
// time each frame is shown and lastDelay the time the last one is shown, in
// hundredths of a second. All frames must have the same size.
func EncodeGIF(frames []image.Image, delay, lastDelay int) ([]byte, error) {
	if len(frames) == 0 {
		return nil, errors.New("no frames to encode")
	}
	rgba := make([]*image.RGBA, len(frames))
	for i, f := range frames {
		rgba[i] = asRGBA(f)
		if rgba[i].Rect.Dx() != rgba[0].Rect.Dx() || rgba[i].Rect.Dy() != rgba[0].Rect.Dy() {
			return nil, fmt.Errorf("frame %d is %v, frame 0 is %v", i, rgba[i].Rect.Size(), rgba[0].Rect.Size())
		}
	}
	w, h := rgba[0].Rect.Dx(), rgba[0].Rect.Dy()
	pal := buildPalette(rgba)
	q := newQuantizer(pal)
	g := &gif.GIF{LoopCount: 0}
	prev, cur := make([]uint8, w*h), make([]uint8, w*h)
	for i, f := range rgba {
		q.indexes(f, cur)
		d := delay
		if i == len(rgba)-1 {
			d = lastDelay
		}
		if i == 0 {
			im := image.NewPaletted(image.Rect(0, 0, w, h), pal)
			copy(im.Pix, cur)
			g.Image, g.Delay, g.Disposal = append(g.Image, im), append(g.Delay, d), append(g.Disposal, gif.DisposalNone)
		} else {
			minX, minY, maxX, maxY := w, h, -1, -1
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if cur[y*w+x] != prev[y*w+x] {
						minX, maxX = min(minX, x), max(maxX, x)
						minY, maxY = min(minY, y), max(maxY, y)
					}
				}
			}
			if maxX < 0 { // identical to the previous frame: show it longer
				g.Delay[len(g.Delay)-1] += d
			} else {
				r := image.Rect(minX, minY, maxX+1, maxY+1)
				im := image.NewPaletted(r, pal)
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						if v := cur[y*w+x]; v != prev[y*w+x] {
							im.Pix[im.PixOffset(x, y)] = v
						} // else 0: transparent, the previous frame shows through
					}
				}
				g.Image, g.Delay, g.Disposal = append(g.Image, im), append(g.Delay, d), append(g.Disposal, gif.DisposalNone)
			}
		}
		prev, cur = cur, prev
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SaveGIF writes frames as an animated GIF to path.
func SaveGIF(path string, frames []image.Image, delay, lastDelay int) error {
	data, err := EncodeGIF(frames, delay, lastDelay)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

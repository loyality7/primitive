package primitive

import (
	"fmt"
	"image"
	"strings"

	"github.com/fogleman/gg"

	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/imageio"
	"github.com/loyality7/primitive/primitive/optimize"
	"github.com/loyality7/primitive/primitive/shape"
)

type Model struct {
	Sw, Sh, Vw, Vh int
	AnimStep       float64        // seconds between shapes in SVG output, 0 = static
	Palette        []colors.Color // restricts shape colors when non-empty
	Blur           float64        // SVG-only gaussian blur radius in image pixels, 0 = off
	Candidates     int            // random shapes tried per hill-climb restart, default 1000
	Restarts       int            // hill-climb restarts per step, default 16
	Scale          float64
	Background     colors.Color
	Target         *image.RGBA
	Current        *image.RGBA
	Context        *gg.Context
	Score          float64
	Shapes         []shape.Shape
	Colors         []colors.Color
	Scores         []float64
	Workers        []*Worker
}

func NewModel(target image.Image, background colors.Color, size, numWorkers int, seed int64) *Model {
	w := target.Bounds().Size().X
	h := target.Bounds().Size().Y
	aspect := float64(w) / float64(h)
	var sw, sh int
	var scale float64
	if aspect >= 1 {
		sw = size
		sh = int(float64(size) / aspect)
		scale = float64(size) / float64(w)
	} else {
		sw = int(float64(size) * aspect)
		sh = size
		scale = float64(size) / float64(h)
	}

	model := &Model{}
	model.Sw = sw
	model.Sh = sh
	model.Vw = w
	model.Vh = h
	model.Scale = scale
	model.Background = background
	model.Target = imageio.ImageToRGBA(target)
	model.Current = imageio.UniformRGBA(target.Bounds(), background.NRGBA())
	model.Score = differenceFull(model.Target, model.Current)
	model.Context = model.newContext()
	model.Candidates, model.Restarts = 1000, 16
	for i := 0; i < numWorkers; i++ {
		worker := NewWorker(model.Target, seed+int64(i))
		model.Workers = append(model.Workers, worker)
	}
	return model
}

func (model *Model) newContext() *gg.Context {
	dc := gg.NewContext(model.Sw, model.Sh)
	dc.Scale(model.Scale, model.Scale)
	dc.Translate(0.5, 0.5)
	dc.SetColor(model.Background.NRGBA())
	dc.Clear()
	return dc
}

func (model *Model) Frames(scoreDelta float64) []image.Image {
	var result []image.Image
	dc := model.newContext()
	result = append(result, imageio.ImageToRGBA(dc.Image()))
	previous := 10.0
	for i, sh := range model.Shapes {
		c := model.Colors[i]
		dc.SetRGBA255(c.R, c.G, c.B, c.A)
		sh.Draw(dc, model.Scale)
		dc.Fill()
		score := model.Scores[i]
		delta := previous - score
		if delta >= scoreDelta {
			previous = score
			result = append(result, imageio.ImageToRGBA(dc.Image()))
		}
	}
	return result
}

func (model *Model) SVG() string {
	bg := model.Background
	fillA := model.Colors[0].A
	var lines []string
	lines = append(lines, fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" version=\"1.1\" viewBox=\"0 0 %d %d\">", model.Vw, model.Vh))
	lines = append(lines, fmt.Sprintf("<rect width=\"100%%\" height=\"100%%\" fill=\"#%02x%02x%02x\" />", bg.R, bg.G, bg.B))
	if model.AnimStep > 0 {
		lines = append(lines, "<style>.a{opacity:0;animation:p .25s ease-out forwards}@keyframes p{to{opacity:1}}</style>")
	}
	group := "<g transform=\"translate(0.5 0.5)\" fill-opacity=\"%f\">"
	if model.Blur > 0 {
		lines = append(lines, fmt.Sprintf("<defs><filter id=\"b\" x=\"-10%%\" y=\"-10%%\" width=\"120%%\" height=\"120%%\"><feGaussianBlur stdDeviation=\"%g\"/></filter></defs>", model.Blur))
		group = "<g transform=\"translate(0.5 0.5)\" fill-opacity=\"%f\" filter=\"url(#b)\">"
	}
	lines = append(lines, fmt.Sprintf(group, float64(fillA)/255))
	for i, sh := range model.Shapes {
		var attrs []string
		c := model.Colors[i]
		fill := "fill=\"#%02x%02x%02x\""
		fill = fmt.Sprintf(fill, c.R, c.G, c.B)
		attrs = append(attrs, fill)
		if c.A != fillA {
			opacity := "fill-opacity=\"%f\""
			attrs = append(attrs, fmt.Sprintf(opacity, float64(c.A)/255))
		}
		if model.AnimStep > 0 {
			attrs = append(attrs, fmt.Sprintf("class=\"a\" style=\"animation-delay:%.3fs\"", float64(i)*model.AnimStep))
		}
		lines = append(lines, sh.SVG(strings.Join(attrs, " ")))
	}
	lines = append(lines, "</g>")
	lines = append(lines, "</svg>")
	return strings.Join(lines, "\n")
}

func (model *Model) Add(sh shape.Shape, alpha int) {
	before := imageio.CopyRGBA(model.Current)
	lines := sh.Rasterize()
	color := computeColor(model.Target, model.Current, lines, alpha, model.Palette)
	drawLines(model.Current, color, lines)
	score := differencePartial(model.Target, before, model.Current, model.Score, lines)

	model.Score = score
	model.Shapes = append(model.Shapes, sh)
	model.Colors = append(model.Colors, color)
	model.Scores = append(model.Scores, score)

	model.Context.SetRGBA255(color.R, color.G, color.B, color.A)
	sh.Draw(model.Context, model.Scale)
}

func (model *Model) Step(shapeType shape.Type, alpha, repeat int) int {
	state := model.runWorkers(shapeType, alpha, model.Candidates, 100, model.Restarts)
	model.Add(state.Shape, state.Alpha)

	for i := 0; i < repeat; i++ {
		state.Worker.Init(model.Current, model.Score)
		a := state.Energy()
		state = optimize.HillClimb(state, 100).(*State)
		b := state.Energy()
		if a == b {
			break
		}
		model.Add(state.Shape, state.Alpha)
	}

	counter := 0
	for _, worker := range model.Workers {
		counter += worker.Counter
	}
	return counter
}

func (model *Model) runWorkers(t shape.Type, a, n, age, m int) *State {
	wn := len(model.Workers)
	ch := make(chan *State, wn)
	wm := m / wn
	if m%wn != 0 {
		wm++
	}
	for i := 0; i < wn; i++ {
		worker := model.Workers[i]
		worker.Init(model.Current, model.Score)
		go model.runWorker(worker, t, a, n, age, wm, ch)
	}
	var bestEnergy float64
	var bestState *State
	for i := 0; i < wn; i++ {
		state := <-ch
		energy := state.Energy()
		if i == 0 || energy < bestEnergy {
			bestEnergy = energy
			bestState = state
		}
	}
	return bestState
}

func (model *Model) runWorker(worker *Worker, t shape.Type, a, n, age, m int, ch chan *State) {
	ch <- worker.BestHillClimbState(t, a, n, age, m)
}

// SetEdgeBias makes new random shapes anchor near high-contrast areas of the
// target with the given probability (0 = off, 1 = always).
func (model *Model) SetEdgeBias(bias float64) {
	if bias <= 0 {
		return
	}
	edge := shape.NewEdgeMap(model.Target)
	for _, w := range model.Workers {
		w.Edge, w.EdgeBias = edge, bias
	}
}

// SetPalette restricts every new shape's color to its nearest palette entry.
// Pass nil to clear.
func (model *Model) SetPalette(palette []colors.Color) {
	model.Palette = palette
	for _, w := range model.Workers {
		w.Palette = palette
	}
}

// SetEffort sets how hard each step searches. Lower values are faster and a
// little less accurate. The defaults are 1000 candidates and 16 restarts.
func (model *Model) SetEffort(candidates, restarts int) {
	if candidates > 0 {
		model.Candidates = candidates
	}
	if restarts > 0 {
		model.Restarts = restarts
	}
}

// Image returns the picture drawn so far.
func (model *Model) Image() image.Image { return model.Context.Image() }

// Search looks for a good next shape with the first worker only and does not
// add it. Several models of the same picture can search at the same time and
// then all add the best shape, which keeps them identical.
func (model *Model) Search(t shape.Type, alpha, restarts int) *State {
	w := model.Workers[0]
	w.Init(model.Current, model.Score)
	return w.BestHillClimbState(t, alpha, model.Candidates, 100, restarts)
}

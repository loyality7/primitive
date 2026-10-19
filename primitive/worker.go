package primitive

import (
	"image"
	"math/rand"

	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/optimize"
	"github.com/loyality7/primitive/primitive/shape"
)

type Worker struct {
	*shape.Env // size, RNG, rasteriser buffer, edge placement
	Target     *image.RGBA
	Current    *image.RGBA
	Buffer     *image.RGBA
	Score      float64
	Counter    int
	Palette    []colors.Color // nil = no color restriction
}

func NewWorker(target *image.RGBA, seed int64) *Worker {
	w := target.Bounds().Size().X
	h := target.Bounds().Size().Y
	return &Worker{
		Env:    shape.NewEnv(w, h, rand.New(rand.NewSource(seed))),
		Target: target,
		Buffer: image.NewRGBA(target.Bounds()),
	}
}

func (worker *Worker) Init(current *image.RGBA, score float64) {
	worker.Current = current
	worker.Score = score
	worker.Counter = 0
}

func (worker *Worker) Energy(sh shape.Shape, alpha int) float64 {
	worker.Counter++
	lines := sh.Rasterize()
	color := computeColor(worker.Target, worker.Current, lines, alpha, worker.Palette)
	copyLines(worker.Buffer, worker.Current, lines)
	drawLines(worker.Buffer, color, lines)
	return differencePartial(worker.Target, worker.Current, worker.Buffer, worker.Score, lines)
}

func (worker *Worker) BestHillClimbState(t shape.Type, a, n, age, m int) *State {
	var bestEnergy float64
	var bestState *State
	for i := 0; i < m; i++ {
		state := worker.BestRandomState(t, a, n)
		before := state.Energy()
		state = optimize.HillClimb(state, age).(*State)
		energy := state.Energy()
		vv("%dx random: %.6f -> %dx hill climb: %.6f\n", n, before, age, energy)
		if i == 0 || energy < bestEnergy {
			bestEnergy = energy
			bestState = state
		}
	}
	return bestState
}

func (worker *Worker) BestRandomState(t shape.Type, a, n int) *State {
	var bestEnergy float64
	var bestState *State
	for i := 0; i < n; i++ {
		state := worker.RandomState(t, a)
		energy := state.Energy()
		if i == 0 || energy < bestEnergy {
			bestEnergy = energy
			bestState = state
		}
	}
	return bestState
}

func (worker *Worker) RandomState(t shape.Type, a int) *State {
	switch t {
	default:
		return worker.RandomState(shape.Type(worker.Rnd.Intn(8)+1), a)
	case shape.TypeTriangle:
		return NewState(worker, shape.NewRandomTriangle(worker.Env), a)
	case shape.TypeRectangle:
		return NewState(worker, shape.NewRandomRectangle(worker.Env), a)
	case shape.TypeEllipse:
		return NewState(worker, shape.NewRandomEllipse(worker.Env), a)
	case shape.TypeCircle:
		return NewState(worker, shape.NewRandomCircle(worker.Env), a)
	case shape.TypeRotatedRectangle:
		return NewState(worker, shape.NewRandomRotatedRectangle(worker.Env), a)
	case shape.TypeQuadratic:
		return NewState(worker, shape.NewRandomQuadratic(worker.Env), a)
	case shape.TypeRotatedEllipse:
		return NewState(worker, shape.NewRandomRotatedEllipse(worker.Env), a)
	case shape.TypePolygon:
		return NewState(worker, shape.NewRandomPolygon(worker.Env, 4, false), a)
	}
}

package primitive

type Annealable interface {
	Energy() float64
	DoMove() interface{}
	UndoMove(interface{})
	Copy() Annealable
}

func HillClimb(state Annealable, maxAge int) Annealable {
	state = state.Copy()
	bestState := state.Copy()
	bestEnergy := state.Energy()
	for age := 0; age < maxAge; age++ {
		undo := state.DoMove()
		energy := state.Energy()
		if energy >= bestEnergy {
			state.UndoMove(undo)
		} else {
			bestEnergy = energy
			bestState = state.Copy()
			age = -1
		}
	}
	return bestState
}

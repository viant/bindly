package state

import "github.com/viant/structology"

type State struct {
	state structology.State
}

func NewState() *State {
	return &State{}
}

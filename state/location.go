package state

type Location struct {
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
	In   string `json:"in,omitempty" yaml:"in,omitempty"`
}

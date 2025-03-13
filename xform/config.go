package xform

type Config struct {
	Name       string            `json:"name,omitempty" yaml:"name,omitempty"`
	Meta       map[string]string `json:"meta,omitempty" yaml:"meta,omitempty"`
	Parameters []string          `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

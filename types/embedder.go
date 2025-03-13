package types

import (
	"embed"
	"reflect"
)

// Embedder represents embedder
type Embedder interface {
	EmbedFS() *embed.FS
}

// FSEmbedder represents fs embedder
type FSEmbedder struct {
	fs       *embed.FS
	embedder Embedder
	rType    reflect.Type
}

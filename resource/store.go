// Package resource provides namespace-scoped access to standard Go filesystems,
// including go:embed values.
package resource

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

type Store struct {
	mu     sync.RWMutex
	byName map[string]fs.FS
}

func New() *Store {
	return &Store{byName: map[string]fs.FS{}}
}

// Register adds a filesystem namespace. An empty name registers the default
// namespace used by references without a prefix.
func (s *Store) Register(name string, source fs.FS) error {
	if s == nil {
		return fmt.Errorf("resource store is required")
	}
	if source == nil {
		return fmt.Errorf("resource filesystem is required")
	}
	name = strings.TrimSpace(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byName[name]; ok {
		return fmt.Errorf("resource namespace %q is already registered", name)
	}
	s.byName[name] = source
	return nil
}

func (s *Store) ReadFile(reference string) ([]byte, error) {
	name, path := splitReference(reference)
	source, name, ok := s.filesystem(name)
	if !ok {
		return nil, fmt.Errorf("resource namespace %q is not registered", name)
	}
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, fmt.Errorf("resource path is required")
	}
	data, err := fs.ReadFile(source, path)
	if err != nil {
		return nil, fmt.Errorf("read resource %q: %w", reference, err)
	}
	return data, nil
}

// Filesystem returns a registered namespace without wrapping the underlying
// fs.FS. Callers that require embed-specific APIs can type-assert the original
// embed.FS value.
func (s *Store) Filesystem(name string) (fs.FS, bool) {
	source, _, ok := s.filesystem(strings.TrimSpace(name))
	return source, ok
}

// Open opens a namespaced resource through the registered filesystem.
func (s *Store) Open(reference string) (fs.File, error) {
	name, path := splitReference(reference)
	source, name, ok := s.filesystem(name)
	if !ok {
		return nil, fmt.Errorf("resource namespace %q is not registered", name)
	}
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, fmt.Errorf("resource path is required")
	}
	file, err := source.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open resource %q: %w", reference, err)
	}
	return file, nil
}

func (s *Store) filesystem(name string) (fs.FS, string, bool) {
	if s == nil {
		return nil, name, false
	}
	s.mu.RLock()
	source, ok := s.byName[name]
	s.mu.RUnlock()
	return source, name, ok
}

func splitReference(reference string) (string, string) {
	reference = strings.TrimSpace(reference)
	if index := strings.IndexByte(reference, ':'); index >= 0 {
		return strings.TrimSpace(reference[:index]), strings.TrimSpace(reference[index+1:])
	}
	return "", reference
}

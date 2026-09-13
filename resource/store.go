// Package resource owns named package filesystems shared by compilation and
// invocation. Registering a named filesystem never creates an implicit default.
package resource

import (
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
)

type Store struct {
	mu        sync.RWMutex
	sources   map[string]fs.FS
	authority *Store
	defaultFS fs.FS
}

func New() *Store { return &Store{sources: map[string]fs.FS{}} }

// WithDefault returns a source-local default view over the same named authority.
// Named Register/Lookup remain shared; registering a default on this view fails
// because its default was fixed at construction. The supplied FS must be immutable.
func (s *Store) WithDefault(source fs.FS) (*Store, error) {
	if s == nil || source == nil {
		return nil, fmt.Errorf("resource filesystem is required")
	}
	authority := s
	if s.authority != nil {
		authority = s.authority
	}
	return &Store{authority: authority, defaultFS: source}, nil
}

func (s *Store) Register(name string, source fs.FS) error {
	if s == nil || source == nil {
		return fmt.Errorf("resource filesystem is required")
	}
	name = strings.TrimSpace(name)
	if strings.ContainsAny(name, ":/\\") {
		return fmt.Errorf("invalid resource namespace %q", name)
	}
	if s.authority != nil {
		if name == "" {
			return fmt.Errorf("scoped resource default is already registered")
		}
		return s.authority.Register(name, source)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sources[name]; exists {
		return fmt.Errorf("resource namespace %q is already registered", name)
	}
	if s.sources == nil {
		s.sources = map[string]fs.FS{}
	}
	s.sources[name] = source
	return nil
}

func (s *Store) Lookup(name string) (fs.FS, bool) {
	if s == nil {
		return nil, false
	}
	if s.authority != nil {
		if name == "" {
			return s.defaultFS, true
		}
		return s.authority.Lookup(name)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.sources[name]
	return source, ok
}

func (s *Store) Open(reference string) (fs.File, error) {
	namespace, path, qualified := strings.Cut(reference, ":")
	if !qualified {
		path = namespace
		namespace = ""
	}
	if !fs.ValidPath(path) {
		return nil, &fs.PathError{Op: "open", Path: reference, Err: fs.ErrInvalid}
	}
	source, ok := s.Lookup(namespace)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: reference, Err: fmt.Errorf("resource namespace %q: %w", namespace, fs.ErrNotExist)}
	}
	return source.Open(path)
}

func (s *Store) ReadFile(reference string) ([]byte, error) {
	file, err := s.Open(reference)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

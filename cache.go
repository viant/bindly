package bindly

import (
	"context"
	"encoding/gob"
	"fmt"
	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/bindly/internal"
	"github.com/viant/structology"
	"io"
	"reflect"
	"sync"
)

type ValueCache struct {
	internal.Map[string, interface{}]
	locker    internal.Map[string, sync.Locker]
	saveMutex sync.Mutex
	fs        afs.Service
}

func (c *ValueCache) lock(key string) sync.Locker {
	locker, ok := c.locker.Get(key)
	if !ok {
		locker = &sync.Mutex{}
		c.locker.Put(key, locker)
	}
	return locker
}

// Save persists the cache to disk
func (c *ValueCache) Save(ctx context.Context, destURL string) error {

	c.saveMutex.Lock()
	defer c.saveMutex.Unlock()

	writer, err := c.fs.NewWriter(ctx, destURL, file.DefaultFileOsMode)
	if err != nil {
		return fmt.Errorf("failed to create file writer: %w", err)
	}
	// Convert cache to serializable format
	serializable := make(map[string]interface{})
	c.Map.Range(func(key string, value interface{}) bool {
		serializable[key] = value
		return true
	})
	// Encode and write to file
	encoder := gob.NewEncoder(writer)
	if err := encoder.Encode(serializable); err != nil {
		return fmt.Errorf("failed to encode cache data: %w", err)
	}
	return writer.Close()
}

// Load loads the cache from disk
func (c *ValueCache) Load(ctx context.Context, URL string) error {
	if ok, _ := c.fs.Exists(ctx, URL); ok {
		return nil
	}
	reader, err := c.fs.OpenURL(ctx, URL)
	if err != nil {
		return err
	}
	defer reader.Close()
	// Decode file contents
	decoder := gob.NewDecoder(reader)
	serialized := make(map[string]interface{})
	if err := decoder.Decode(&serialized); err != nil {
		if err == io.EOF {
			return nil // Empty file
		}
		return fmt.Errorf("failed to decode cache file: %w", err)
	}
	// Update cache with loaded data
	for key, value := range serialized {
		c.Map.Put(key, value)
	}
	return nil
}

// Clear empties the cache
func (c *ValueCache) Clear() {
	c.Map = internal.NewMap[string, interface{}]()
	c.locker = internal.NewMap[string, sync.Locker]()
}

func NewValueCache() *ValueCache {
	return &ValueCache{Map: internal.NewMap[string, interface{}](), fs: afs.New(), locker: internal.NewMap[string, sync.Locker]()}
}

type BindingCache struct {
	internal.Map[reflect.Type, *BindingType]
}

func NewBindingCache() *BindingCache {
	return &BindingCache{Map: internal.NewMap[reflect.Type, *BindingType]()}
}

type StructTypeCache struct {
	internal.Map[reflect.Type, *structology.StateType]
}

func NewStructTypeCache() *StructTypeCache {
	return &StructTypeCache{Map: internal.NewMap[reflect.Type, *structology.StateType]()}
}

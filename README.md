Bindaly - Golang Dependency Injection

[![GoReportCard](https://goreportcard.com/badge/github.com/viant/binder)](https://goreportcard.com/report/github.com/viant/binder)
[![GoDoc](https://godoc.org/github.com/viant/binder?status.svg)](https://godoc.org/github.com/viant/binder)

Bindaly is a powerful, flexible dependency injection library for Go that helps manage application component dependencies with minimal boilerplate and maximal type safety.

## Introduction

Bindaly provides a straightforward way to inject dependencies into Go structs using struct tags. It works with generic types to provide both flexibility and type safety, and supports caching, transformations, and complex dependency graphs.

## Features

- Type-safe dependency injection with generics
- Annotation-based binding with struct tags
- Value transformations via custom transformers
- Intelligent type conversion between compatible types
- Value caching for performance optimization
- Fully concurrent-safe operations
- Extensible architecture with custom providers and locators

## Installation

```bash
go get github.com/viant/binder
```

## Usage

### Basic Example

```go
package main

import (
	"context"
	"fmt"
	"github.com/viant/bindly"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/structology"
)

type AppConfig struct {
	ServerPort int
	BaseURL    string
}

// Logger is a simple logging service
type Logger interface {
	Log(message string)
}

// SimpleLogger implements Logger
type SimpleLogger struct{}

func (l *SimpleLogger) Log(message string) {
	fmt.Println(message)
}

type DependencySetup struct {
	Config     *AppConfig
	Settings   map[string]interface{}
	Interfaces map[string]interface{}
}

// Service uses the configuration
type Service struct {
	Debug      bool   `bind:"kind=setting,in=debug"`
	ServerPort int    `bind:"in=Config.ServerPort"` //state kind is a default kind
	Logger     Logger //interface bind by default to interface kind
}

func main() {

	var iLogger Logger
	appLoger := &SimpleLogger{}

	setup := &DependencySetup{
		Config: &AppConfig{
			ServerPort: 8080,
			BaseURL:    "http://localhost:8080",
		},

		Interfaces: map[string]interface{}{
			structology.InterfaceTypeOf(&iLogger).String(): appLoger,
		},
		Settings: map[string]interface{}{
			"debug": true,
			"port":  8080,
		},
	}

	var opts = append([]bindly.InjectorOption{}, bindly.WithProviders(
		buildin.Struct("state", "", 1),
		buildin.Map("setting", "Settings", 1),
		buildin.Map("interface", "Interfaces", 1)))

	injector := bindly.NewInjector(opts...)
	service := &Service{}
	err := bindly.WithState[Service](injector, setup).Inject(context.Background(), service)
	fmt.Println(service, err)
}

```

### Binding with Tags

Bindaly uses struct tags to define dependencies:

```go
type MyStruct struct {
    // Inject from state with key "config.port"
    Port int `bind:"in=config.port"`
    
    // Inject from a specific provider kind
    Database *Database `bind:"kind=database,in=primary"`
    
    // Cache the resolved value
    ExpensiveData []Item `bind:"kind=service,in=data,cacheable"`
    
    // Transform values during injection
    ConfigValue string `bind:"in=rawValue" xform:"string"`
}
```

### Value Transformers

Transformers convert values during injection:

```go
// Register custom transformers
transformerRegistry := xform.NewRegistry()
xform.conv.Init(transformerRegistry)  // Initialize standard converters
transformerRegistry.Register("custom", xform.NewTransformerFactory("custom", NewCustomTransformer))

injector := bindly.New(
    bindly.WithTransformers(transformerRegistry),
)
```

## Advanced Features

### Caching Values

```go
// Create a value cache
cache := bindly.NewValueCache()

// Use cache with binding context
bindingCtx := bindly.WithState[MyService](
    injector, 
    state,
    bindly.WithCache[MyService](cache),
)

// Save cache to disk
err := cache.Save(ctx, "/path/to/cache.bin")

// Load cache from disk
err := cache.Load(ctx, "/path/to/cache.bin")
```

### Custom Providers

```go
// Implement custom provider
type MyProvider struct {
    // provider implementation
}

func (p *MyProvider) Locate(state *structology.State) locator.Locator {
    // Return appropriate locator based on state
}

// Register provider
injector := bindly.New(
    bindly.WithProviders(&MyProvider{}),
)
```

## Performance Considerations

- Use `cacheable` for expensive operations
- Prefer direct binding over transformations when possible
- Consider pre-building binding types for frequently used structs

## Thread Safety

The binder library is designed to be fully thread-safe. All caching operations use appropriate locking mechanisms to ensure safe concurrent access.

## License

The source code is made available under the terms of the Apache License, Version 2.0. See the [LICENSE](LICENSE) file for more details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

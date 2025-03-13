package bindly_test

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
	Logger     Logger //interface bind by defaul to interface kind
}

func Example_inject() {

	var iLogger Logger
	appLoger := &SimpleLogger{}

	dependencies := &DependencySetup{
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
		buildin.Struct("state", "", 1), //empty kind
		buildin.Map("setting", "Settings", 1),
		buildin.Map("interface", "Interfaces", 1)))

	injector := bindly.NewInjector(opts...)
	service := &Service{}
	err := bindly.WithState[Service](injector, dependencies).Inject(context.Background(), service)

	fmt.Println(service, err)
}

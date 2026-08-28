package environment

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
)

//go:generate go tool -modfile=../../tools/go.mod envdoc -output ../../docs/environment.md -format markdown -types Config
//go:generate go run ./cmd/trimdoc ../../docs/environment.md
type Config struct {
	// Enforce Manja's conservative catalog source, compilation, startup, storage, and HTML rendering budgets.
	ResourceLimits bool `env:"MANJA_RESOURCE_LIMITS" envDefault:"false"`
	// Enable or disable local docs in the recovery-only runtime. Accepted values are on and off.
	LocalDocs string `env:"MANJA_LOCAL_DOCS" envDefault:"on"`
}

func Parse(values map[string]string) (Config, error) {
	if value, exists := values["MANJA_LOCAL_DOCS"]; exists && value == "" {
		return Config{}, fmt.Errorf("MANJA_LOCAL_DOCS must be on or off")
	}
	config, err := env.ParseAsWithOptions[Config](env.Options{Environment: values})
	if err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}
	if config.LocalDocs != "on" && config.LocalDocs != "off" {
		return Config{}, fmt.Errorf("MANJA_LOCAL_DOCS must be on or off")
	}
	return config, nil
}

func Load() (Config, error) {
	return Parse(env.ToMap(os.Environ()))
}

func (config Config) LocalDocsDisabled() bool {
	return config.LocalDocs == "off"
}

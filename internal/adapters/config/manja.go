// Package config loads repository-owned Manja contract-review configuration.
package config

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	core "github.com/araihu/manja/domain"
	"gopkg.in/yaml.v3"
)

// File is the versioned repository configuration document.
type File struct {
	Version   int                       `yaml:"version"`
	Contracts map[string]ContractConfig `yaml:"contracts"`
}

// ContractConfig configures one OpenAPI contract and its policy profiles.
type ContractConfig struct {
	Spec          string                  `yaml:"spec"`
	DefaultPolicy string                  `yaml:"defaultPolicy"`
	Policies      map[string]PolicyConfig `yaml:"policies"`
}

// PolicyConfig is a repository-owned policy profile.
type PolicyConfig struct {
	RequireReleaseBaseline bool              `yaml:"requireReleaseBaseline"`
	Rules                  map[string]string `yaml:"rules"`
	Exceptions             []ExceptionConfig `yaml:"exceptions"`
}

// ExceptionConfig is a time-bounded policy exception.
type ExceptionConfig struct {
	Finding string `yaml:"finding"`
	Rule    string `yaml:"rule"`
	Reason  string `yaml:"reason"`
	Author  string `yaml:"author"`
	Expires string `yaml:"expires"`
}

// Load reads one strict, versioned Manja configuration document.
func Load(path string) (File, error) {
	input, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("open config: %w", err)
	}
	defer input.Close()

	decoder := yaml.NewDecoder(input)
	decoder.KnownFields(true)
	var file File
	if err := decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return File{}, fmt.Errorf("config must contain exactly one document")
		}
		return File{}, fmt.Errorf("decode config: %w", err)
	}
	if err := file.validate(); err != nil {
		return File{}, err
	}
	return file, nil
}

// Contract returns the named contract configuration.
func (f File) Contract(id string) (ContractConfig, error) {
	contract, ok := f.Contracts[id]
	if !ok || strings.TrimSpace(id) == "" {
		return ContractConfig{}, fmt.Errorf("contract %q is not configured", id)
	}
	return contract, nil
}

// PolicyLayer returns the named policy profile as a repository policy layer.
// An empty name selects the contract's default policy profile.
func (c ContractConfig) PolicyLayer(name string) (core.PolicyLayer, error) {
	if strings.TrimSpace(name) == "" {
		name = c.DefaultPolicy
	}
	profile, ok := c.Policies[name]
	if !ok || strings.TrimSpace(name) == "" {
		return core.PolicyLayer{}, fmt.Errorf("policy profile %q is not configured", name)
	}

	layer := core.PolicyLayer{
		Name:                   name,
		Source:                 core.PolicySourceRepository,
		RequireReleaseBaseline: profile.RequireReleaseBaseline,
		Rules:                  make(map[string]core.RuleLevel, len(profile.Rules)),
		Exceptions:             make([]core.PolicyException, 0, len(profile.Exceptions)),
	}
	for ruleID, level := range profile.Rules {
		layer.Rules[ruleID] = core.RuleLevel(level)
	}
	for index, configured := range profile.Exceptions {
		exception, err := configured.policyException()
		if err != nil {
			return core.PolicyLayer{}, fmt.Errorf("policy profile %q exception %d: %w", name, index, err)
		}
		layer.Exceptions = append(layer.Exceptions, exception)
	}

	if _, err := core.MergePolicy(layer); err != nil {
		return core.PolicyLayer{}, err
	}
	return layer, nil
}

func (f File) validate() error {
	if f.Version != 1 {
		return fmt.Errorf("config version 1 is required")
	}
	if len(f.Contracts) == 0 {
		return fmt.Errorf("at least one contract is required")
	}
	for id, contract := range f.Contracts {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("contract id is required")
		}
		if strings.TrimSpace(contract.Spec) == "" {
			return fmt.Errorf("contract %q spec is required", id)
		}
		if strings.TrimSpace(contract.DefaultPolicy) == "" {
			return fmt.Errorf("contract %q default policy is required", id)
		}
		if len(contract.Policies) == 0 {
			return fmt.Errorf("contract %q requires a policy profile", id)
		}
		for name := range contract.Policies {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("contract %q policy profile name is required", id)
			}
			if _, err := contract.PolicyLayer(name); err != nil {
				return fmt.Errorf("contract %q: %w", id, err)
			}
		}
		if _, err := contract.PolicyLayer(""); err != nil {
			return fmt.Errorf("contract %q default policy: %w", id, err)
		}
	}
	return nil
}

func (e ExceptionConfig) policyException() (core.PolicyException, error) {
	if strings.TrimSpace(e.Expires) == "" {
		return core.PolicyException{}, fmt.Errorf("expiry is required")
	}
	expiresAt, err := time.Parse(time.RFC3339, e.Expires)
	if err != nil {
		return core.PolicyException{}, fmt.Errorf("expiry must be RFC3339: %w", err)
	}
	return core.PolicyException{
		FindingID: e.Finding,
		RuleID:    e.Rule,
		Reason:    e.Reason,
		Author:    e.Author,
		ExpiresAt: expiresAt.UTC(),
		Source:    core.PolicySourceRepository,
	}, nil
}

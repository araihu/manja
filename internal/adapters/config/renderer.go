package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/renderer"
	"gopkg.in/yaml.v3"
)

const (
	RendererSourceFiles = "files"
	RendererSourceGit   = "git"
)

type RendererFile struct {
	Version  uint32                  `yaml:"version"`
	Catalogs []RendererCatalogConfig `yaml:"catalogs"`
}

type RendererCatalogConfig struct {
	ID                 string               `yaml:"id"`
	Mount              string               `yaml:"mount"`
	Title              string               `yaml:"title"`
	DefaultDocumentKey string               `yaml:"defaultDocument"`
	ProfileID          string               `yaml:"profile"`
	Source             RendererSourceConfig `yaml:"source"`
}

type RendererSourceConfig struct {
	Kind       string   `yaml:"kind"`
	Root       string   `yaml:"root"`
	Include    []string `yaml:"include"`
	Repository string   `yaml:"repository"`
	Ref        string   `yaml:"ref"`
}

func LoadRenderer(filename string) (RendererFile, error) {
	input, err := os.Open(filename)
	if err != nil {
		return RendererFile{}, fmt.Errorf("open renderer config: %w", err)
	}
	defer input.Close()

	decoder := yaml.NewDecoder(input)
	decoder.KnownFields(true)
	var file RendererFile
	if err := decoder.Decode(&file); err != nil {
		return RendererFile{}, fmt.Errorf("decode renderer config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return RendererFile{}, fmt.Errorf("renderer config must contain exactly one document")
		}
		return RendererFile{}, fmt.Errorf("decode renderer config: %w", err)
	}
	if err := file.validate(); err != nil {
		return RendererFile{}, err
	}
	return file, nil
}

func (file RendererFile) RuntimeConfig() renderer.Config {
	result := renderer.Config{Version: file.Version, Catalogs: make([]renderer.CatalogConfig, len(file.Catalogs))}
	for index, catalog := range file.Catalogs {
		result.Catalogs[index] = renderer.CatalogConfig{
			ID: catalog.ID, Mount: catalog.Mount, Title: catalog.Title,
			DefaultDocumentKey: catalog.DefaultDocumentKey,
			ProfileID:          domain.CompatibilityProfileID(catalog.ProfileID),
		}
	}
	return result
}

func (file RendererFile) validate() error {
	if _, err := renderer.New(file.RuntimeConfig()); err != nil {
		return err
	}
	for _, catalog := range file.Catalogs {
		if err := catalog.Source.validate(); err != nil {
			return fmt.Errorf("catalog %q source: %w", catalog.ID, err)
		}
		if len(catalog.Source.Include) == 0 {
			return fmt.Errorf("catalog %q source include is required", catalog.ID)
		}
		for includeIndex, include := range catalog.Source.Include {
			if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("catalog %q include %d", catalog.ID, includeIndex), include, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (source RendererSourceConfig) validate() error {
	switch source.Kind {
	case RendererSourceFiles:
		if strings.TrimSpace(source.Root) == "" {
			return fmt.Errorf("file root is required")
		}
		if source.Repository != "" || source.Ref != "" {
			return fmt.Errorf("file source must not contain Git fields")
		}
	case RendererSourceGit:
		if strings.TrimSpace(source.Repository) == "" || strings.TrimSpace(source.Ref) == "" {
			return fmt.Errorf("Git repository and ref are required")
		}
		if source.Root != "" {
			return fmt.Errorf("Git source must not contain a file root")
		}
		repository, err := url.Parse(source.Repository)
		if err != nil || repository.User != nil || repository.RawQuery != "" || repository.Fragment != "" {
			return fmt.Errorf("Git repository must not contain embedded credentials, query, or fragment")
		}
	default:
		return fmt.Errorf("source kind %q is unsupported", source.Kind)
	}
	return nil
}

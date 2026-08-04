package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/domain"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	"github.com/araihu/manja/renderer"
	"gopkg.in/yaml.v3"
)

const (
	RendererSourceFiles = "files"
	RendererSourceGit   = "git"
)

type RendererFile struct {
	Version  uint32                  `yaml:"version"`
	DataDir  string                  `yaml:"dataDir"`
	Catalogs []RendererCatalogConfig `yaml:"catalogs"`

	baseDir string
}

type RendererCatalogConfig struct {
	ID                     string               `yaml:"id"`
	Mount                  string               `yaml:"mount"`
	Title                  string               `yaml:"title"`
	DefaultDocumentKey     string               `yaml:"defaultDocument"`
	ProfileID              string               `yaml:"profile"`
	CompatibilityAllowlist string               `yaml:"compatibilityAllowlist"`
	Source                 RendererSourceConfig `yaml:"source"`
	SEO                    RendererSEOConfig    `yaml:"seo"`

	compatibilityAllowlist []byte
}

type RendererSEOConfig struct {
	Description    string `yaml:"description"`
	CanonicalBase  string `yaml:"canonicalBase"`
	SocialImage    string `yaml:"socialImage"`
	SocialImageAlt string `yaml:"socialImageAlt"`
}

type RendererSourceConfig struct {
	Kind       string                   `yaml:"kind"`
	Root       string                   `yaml:"root"`
	Include    []string                 `yaml:"include"`
	Documents  []RendererSourceDocument `yaml:"documents"`
	Repository string                   `yaml:"repository"`
	Ref        string                   `yaml:"ref"`
}

type RendererSourceDocument struct {
	Path string `yaml:"path"`
	Key  string `yaml:"key"`
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
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return RendererFile{}, fmt.Errorf("resolve renderer config: %w", err)
	}
	file.baseDir = filepath.Dir(absolute)
	for index := range file.Catalogs {
		path := strings.TrimSpace(file.Catalogs[index].CompatibilityAllowlist)
		if path == "" {
			continue
		}
		contents, err := os.ReadFile(file.resolve(path))
		if err != nil {
			return RendererFile{}, fmt.Errorf("catalog %q compatibility allowlist: %w", file.Catalogs[index].ID, err)
		}
		file.Catalogs[index].compatibilityAllowlist = contents
	}
	if err := file.validate(); err != nil {
		return RendererFile{}, err
	}
	return file, nil
}

func (file RendererFile) RuntimeConfig() renderer.Config {
	result := renderer.Config{Version: file.Version, DataDir: file.resolve(file.DataDir), Catalogs: make([]renderer.CatalogConfig, len(file.Catalogs))}
	for index, catalog := range file.Catalogs {
		result.Catalogs[index] = renderer.CatalogConfig{
			ID: catalog.ID, Mount: catalog.Mount, Title: catalog.Title,
			DefaultDocumentKey:     catalog.DefaultDocumentKey,
			ProfileID:              domain.CompatibilityProfileID(catalog.ProfileID),
			CompatibilityAllowlist: append([]byte(nil), catalog.compatibilityAllowlist...),
			SEO: renderer.CatalogSEO{
				Description: catalog.SEO.Description, CanonicalBase: catalog.SEO.CanonicalBase,
				SocialImage: catalog.SEO.SocialImage, SocialImageAlt: catalog.SEO.SocialImageAlt,
			},
		}
	}
	return result
}

func (file RendererFile) Sources() []renderer.CatalogSource {
	result := make([]renderer.CatalogSource, len(file.Catalogs))
	for index, configured := range file.Catalogs {
		manifest := sourceadapter.CatalogManifest{
			ID: configured.ID, Title: configured.Title,
			DefaultDocumentKey: configured.DefaultDocumentKey,
			ProfileID:          domain.CompatibilityProfileID(configured.ProfileID),
			Includes:           append([]string(nil), configured.Source.Include...),
			DocumentKeys:       make([]sourceadapter.CatalogDocumentKey, len(configured.Source.Documents)),
		}
		for documentIndex, document := range configured.Source.Documents {
			manifest.DocumentKeys[documentIndex] = sourceadapter.CatalogDocumentKey{SourcePath: document.Path, Key: document.Key}
		}
		switch configured.Source.Kind {
		case RendererSourceFiles:
			result[index] = sourceadapter.FileCatalogSource{Root: file.resolve(configured.Source.Root), Manifest: manifest}
		case RendererSourceGit:
			result[index] = sourceadapter.GitCatalogSource{
				Repository: configured.Source.Repository, Ref: configured.Source.Ref, Manifest: manifest,
			}
		}
	}
	return result
}

func (file RendererFile) resolve(path string) string {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) || file.baseDir == "" {
		return path
	}
	return filepath.Clean(filepath.Join(file.baseDir, path))
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
	seenPaths := make(map[string]struct{}, len(source.Documents))
	seenKeys := make(map[string]struct{}, len(source.Documents))
	for index, document := range source.Documents {
		if document.Path == "" || strings.HasPrefix(document.Path, "/") || strings.Contains(document.Path, `\`) || path.Clean(document.Path) != document.Path || document.Path == "." || strings.HasPrefix(document.Path, "../") {
			return fmt.Errorf("document mapping %d path %q is not a clean relative slash path", index, document.Path)
		}
		if err := domain.ValidateCatalogDocumentKey(document.Key); err != nil {
			return err
		}
		if _, exists := seenPaths[document.Path]; exists {
			return fmt.Errorf("document mapping path %q is duplicated", document.Path)
		}
		if _, exists := seenKeys[document.Key]; exists {
			return fmt.Errorf("document mapping key %q is duplicated", document.Key)
		}
		seenPaths[document.Path] = struct{}{}
		seenKeys[document.Key] = struct{}{}
	}
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

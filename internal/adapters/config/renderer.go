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
	"github.com/araihu/manja/renderer"
	"gopkg.in/yaml.v3"
)

const (
	RendererSourceFiles = "files"
	RendererSourceGit   = "git"
)

type RendererFile struct {
	Version      uint32                     `yaml:"version"`
	DataDir      string                     `yaml:"dataDir"`
	Organization RendererOrganizationConfig `yaml:"organization"`
	Catalogs     []RendererCatalogConfig    `yaml:"catalogs"`

	baseDir string
}

type RendererOrganizationConfig struct {
	Title   string                             `yaml:"title"`
	Readme  string                             `yaml:"readme"`
	License RendererOrganizationLicenseConfig  `yaml:"license"`
	Sources []RendererOrganizationSourceConfig `yaml:"sources"`
	SEO     RendererSEOConfig                  `yaml:"seo"`
}

type RendererOrganizationLicenseConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type RendererOrganizationSourceConfig struct {
	Name     string `yaml:"name"`
	Kind     string `yaml:"kind"`
	Location string `yaml:"location"`
	URL      string `yaml:"url"`
}

type RendererCatalogConfig struct {
	ID                     string                       `yaml:"id"`
	Mount                  string                       `yaml:"mount"`
	Title                  string                       `yaml:"title"`
	Readme                 string                       `yaml:"readme"`
	License                RendererCatalogLicenseConfig `yaml:"license"`
	Branding               RendererBrandingConfig       `yaml:"branding"`
	DefaultDocumentKey     string                       `yaml:"defaultDocument"`
	ProfileID              string                       `yaml:"profile"`
	CompatibilityAllowlist string                       `yaml:"compatibilityAllowlist"`
	Source                 RendererSourceConfig         `yaml:"source"`
	SEO                    RendererSEOConfig            `yaml:"seo"`
	LocalDocs              RendererCatalogLocalDocs     `yaml:"localDocs"`

	compatibilityAllowlist []byte
}

type RendererCatalogLocalDocs struct {
	Public         bool   `yaml:"public"`
	Anonymous      bool   `yaml:"anonymous"`
	PublicationKey string `yaml:"publicationKey"`
}

type RendererCatalogLicenseConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// RendererBrandingConfig is presentation metadata carried by the catalog
// source manifest. It stays separate from renderer routing so source-backed
// snapshots can retain the same brand when they are published elsewhere.
type RendererBrandingConfig struct {
	DisplayName string                     `yaml:"displayName"`
	Logo        RendererBrandingLogoConfig `yaml:"logo"`
	Favicon     string                     `yaml:"favicon"`
}

type RendererBrandingLogoConfig struct {
	Src     string `yaml:"src"`
	Alt     string `yaml:"alt"`
	HomeURL string `yaml:"homeUrl"`
}

type RendererSEOConfig struct {
	Description    string `yaml:"description"`
	CanonicalBase  string `yaml:"canonicalBase"`
	SocialImage    string `yaml:"socialImage"`
	SocialImageAlt string `yaml:"socialImageAlt"`
}

type RendererSourceConfig struct {
	Kind             string                   `yaml:"kind"`
	Root             string                   `yaml:"root"`
	Include          []string                 `yaml:"include"`
	Documents        []RendererSourceDocument `yaml:"documents"`
	Repository       string                   `yaml:"repository"`
	Ref              string                   `yaml:"ref"`
	IntegrityReceipt *string                  `yaml:"integrityReceipt"`
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
	result := renderer.Config{
		Version: file.Version,
		DataDir: file.resolve(file.DataDir),
		Organization: renderer.OrganizationConfig{
			Title:   file.Organization.Title,
			Readme:  file.Organization.Readme,
			License: renderer.OrganizationLicense{Name: file.Organization.License.Name, URL: file.Organization.License.URL},
			Sources: make([]renderer.OrganizationSource, len(file.Organization.Sources)),
			SEO: renderer.CatalogSEO{
				Description: file.Organization.SEO.Description, CanonicalBase: file.Organization.SEO.CanonicalBase,
				SocialImage: file.Organization.SEO.SocialImage, SocialImageAlt: file.Organization.SEO.SocialImageAlt,
			},
		},
		Catalogs: make([]renderer.CatalogConfig, len(file.Catalogs)),
	}
	for index, source := range file.Organization.Sources {
		result.Organization.Sources[index] = renderer.OrganizationSource{
			Name: source.Name, Kind: source.Kind, Location: source.Location, URL: source.URL,
		}
	}
	for index, catalog := range file.Catalogs {
		result.Catalogs[index] = renderer.CatalogConfig{
			ID: catalog.ID, Mount: catalog.Mount, Title: catalog.Title, Readme: catalog.Readme,
			License:                renderer.CatalogLicense{Name: catalog.License.Name, URL: catalog.License.URL},
			DefaultDocumentKey:     catalog.DefaultDocumentKey,
			ProfileID:              domain.CompatibilityProfileID(catalog.ProfileID),
			CompatibilityAllowlist: append([]byte(nil), catalog.compatibilityAllowlist...),
			SEO: renderer.CatalogSEO{
				Description: catalog.SEO.Description, CanonicalBase: catalog.SEO.CanonicalBase,
				SocialImage: catalog.SEO.SocialImage, SocialImageAlt: catalog.SEO.SocialImageAlt,
			},
			LocalDocs: renderer.CatalogLocalDocs{
				Public: catalog.LocalDocs.Public, Anonymous: catalog.LocalDocs.Anonymous, PublicationKey: catalog.LocalDocs.PublicationKey,
			},
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
	if err := renderer.ValidateConfig(file.RuntimeConfig()); err != nil {
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
		if source.Repository != "" || source.Ref != "" || source.IntegrityReceipt != nil {
			return fmt.Errorf("file source must not contain Git fields")
		}
	case RendererSourceGit:
		if strings.TrimSpace(source.Repository) == "" || strings.TrimSpace(source.Ref) == "" {
			return fmt.Errorf("Git repository and ref are required")
		}
		if source.Root != "" {
			return fmt.Errorf("Git source must not contain a file root")
		}
		if source.IntegrityReceipt != nil {
			if err := validateRendererIntegrityReceiptPath(*source.IntegrityReceipt); err != nil {
				return err
			}
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

func validateRendererIntegrityReceiptPath(receipt string) error {
	if receipt == "" || strings.TrimSpace(receipt) != receipt || strings.HasPrefix(receipt, "/") || strings.Contains(receipt, `\`) || strings.ContainsRune(receipt, 0) || hasWindowsDrivePrefix(receipt) || receipt == "." || path.Clean(receipt) != receipt || strings.HasPrefix(receipt, "../") {
		return fmt.Errorf("Git integrity receipt %q is not a clean relative slash path", receipt)
	}
	return nil
}

func hasWindowsDrivePrefix(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	return value[0] >= 'a' && value[0] <= 'z' || value[0] >= 'A' && value[0] <= 'Z'
}

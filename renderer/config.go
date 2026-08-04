package renderer

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/araihu/manja/domain"
)

const maxConfiguredCatalogs = 8

const DefaultStartupProcessBytes = uint64(512 << 20)

type Config struct {
	Version             uint32
	DataDir             string
	StartupProcessBytes uint64
	Catalogs            []CatalogConfig
}

type CatalogConfig struct {
	ID                     string
	Mount                  string
	Title                  string
	DefaultDocumentKey     string
	ProfileID              domain.CompatibilityProfileID
	CompatibilityAllowlist []byte
	SEO                    CatalogSEO
}

type CatalogSEO struct {
	Description    string
	CanonicalBase  string
	SocialImage    string
	SocialImageAlt string
}

func validateConfig(config Config) error {
	if config.Version != 1 {
		return fmt.Errorf("renderer config version 1 is required")
	}
	if len(config.Catalogs) == 0 {
		return fmt.Errorf("renderer requires at least one catalog")
	}
	if len(config.Catalogs) > maxConfiguredCatalogs {
		return fmt.Errorf("renderer catalogs exceed %d", maxConfiguredCatalogs)
	}
	ids := make(map[string]struct{}, len(config.Catalogs))
	for index, catalog := range config.Catalogs {
		if err := domain.ValidateCatalogID(catalog.ID); err != nil {
			return fmt.Errorf("catalog %d: %w", index, err)
		}
		if _, exists := ids[catalog.ID]; exists {
			return fmt.Errorf("catalog id %q is duplicated", catalog.ID)
		}
		ids[catalog.ID] = struct{}{}
		if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("catalog %q title", catalog.ID), catalog.Title, false); err != nil {
			return err
		}
		if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("catalog %q profile", catalog.ID), string(catalog.ProfileID), false); err != nil {
			return err
		}
		if catalog.DefaultDocumentKey != "" {
			if err := domain.ValidateCatalogDocumentKey(catalog.DefaultDocumentKey); err != nil {
				return fmt.Errorf("catalog %q default document: %w", catalog.ID, err)
			}
		}
		if err := validateMount(catalog.Mount); err != nil {
			return fmt.Errorf("catalog %q mount: %w", catalog.ID, err)
		}
		if err := validateCatalogSEO(catalog.ID, catalog.SEO); err != nil {
			return err
		}
	}
	for left := range config.Catalogs {
		for right := left + 1; right < len(config.Catalogs); right++ {
			if mountsOverlap(config.Catalogs[left].Mount, config.Catalogs[right].Mount) {
				return fmt.Errorf("catalog mounts %q and %q overlap", config.Catalogs[left].Mount, config.Catalogs[right].Mount)
			}
		}
	}
	return nil
}

// ValidateConfig checks renderer routing and presentation configuration without
// constructing parsers, compilers, storage, or source adapters.
func ValidateConfig(config Config) error {
	if config.StartupProcessBytes == 0 {
		config.StartupProcessBytes = DefaultStartupProcessBytes
	}
	return validateConfig(config)
}

func validateCatalogSEO(catalogID string, seo CatalogSEO) error {
	for label, value := range map[string]string{"description": seo.Description, "social image alt": seo.SocialImageAlt} {
		if value != "" {
			if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("catalog %q SEO %s", catalogID, label), value, false); err != nil {
				return err
			}
		}
	}
	for label, value := range map[string]string{"canonical base": seo.CanonicalBase, "social image": seo.SocialImage} {
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("catalog %q SEO %s must be an absolute HTTPS URL without credentials, query, or fragment", catalogID, label)
		}
	}
	if seo.SocialImage != "" && seo.SocialImageAlt == "" {
		return fmt.Errorf("catalog %q SEO social image alt is required with social image", catalogID)
	}
	if seo.SocialImage != "" && socialImageMIMEType(seo.SocialImage) == "" {
		return fmt.Errorf("catalog %q SEO social image must use a supported .png, .jpg, .jpeg, or .webp extension", catalogID)
	}
	return nil
}

func socialImageMIMEType(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(path.Ext(parsed.Path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func validateMount(mount string) error {
	if err := domain.ValidateCanonicalIdentity("mount", mount, false); err != nil {
		return err
	}
	if mount == "/" {
		return nil
	}
	if !strings.HasPrefix(mount, "/") || strings.HasSuffix(mount, "/") || strings.Contains(mount, `\`) || strings.ContainsAny(mount, "?#%") || path.Clean(mount) != mount {
		return fmt.Errorf("must be an absolute clean unencoded path without a trailing slash")
	}
	for _, reserved := range []string{"/_manja", "/assets", "/manja-assets", "/healthz", "/readyz"} {
		if mount == reserved || strings.HasPrefix(mount, reserved+"/") {
			return fmt.Errorf("uses reserved prefix %q", reserved)
		}
	}
	return nil
}

func mountsOverlap(left, right string) bool {
	return left == "/" || right == "/" || left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

package renderer

import (
	"fmt"
	"path"
	"strings"

	"github.com/araihu/manja/domain"
)

const maxConfiguredCatalogs = 8

type Config struct {
	Version  uint32
	DataDir  string
	Catalogs []CatalogConfig
}

type CatalogConfig struct {
	ID                     string
	Mount                  string
	Title                  string
	DefaultDocumentKey     string
	ProfileID              domain.CompatibilityProfileID
	CompatibilityAllowlist []byte
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

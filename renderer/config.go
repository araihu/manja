package renderer

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/araihu/manja/domain"
)

const (
	maxConfiguredCatalogs      = 8
	maxCatalogReadmeBytes      = 64 << 10
	maxOrganizationSources     = 32
	maxOrganizationReadmeBytes = 64 << 10
	OrganizationSourceKindGit  = "git"
	OrganizationSourceKindFile = "file"
)

const DefaultStartupProcessBytes = uint64(512 << 20)

type Config struct {
	Version             uint32
	DataDir             string
	StartupProcessBytes uint64
	LocalDocsDisabled   bool
	Organization        OrganizationConfig
	Catalogs            []CatalogConfig
}

// OrganizationConfig describes the optional renderer root. Sources are
// intentionally explicit: Manja never derives or exposes private acquisition
// locations from catalog source adapters.
type OrganizationConfig struct {
	Title   string
	Readme  string
	License OrganizationLicense
	Sources []OrganizationSource
	SEO     CatalogSEO
}

type OrganizationLicense struct {
	Name string
	URL  string
}

type OrganizationSource struct {
	Name     string
	Kind     string
	Location string
	URL      string
}

type CatalogConfig struct {
	ID                     string
	Mount                  string
	Title                  string
	Readme                 string
	License                CatalogLicense
	DefaultDocumentKey     string
	ProfileID              domain.CompatibilityProfileID
	CompatibilityAllowlist []byte
	SEO                    CatalogSEO
	LocalDocs              CatalogLocalDocs
}

type CatalogLocalDocs struct {
	Public         bool
	Anonymous      bool
	PublicationKey string
}

type CatalogLicense struct {
	Name string
	URL  string
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
	if err := validateOrganization(config.Organization); err != nil {
		return err
	}
	ids := make(map[string]struct{}, len(config.Catalogs))
	publicationKeys := make(map[string]string, len(config.Catalogs))
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
		if len(catalog.Readme) > maxCatalogReadmeBytes {
			return fmt.Errorf("catalog %q README exceeds %d bytes", catalog.ID, maxCatalogReadmeBytes)
		}
		if catalog.Readme != "" {
			if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("catalog %q README", catalog.ID), catalog.Readme, false); err != nil {
				return err
			}
		}
		if catalog.License.Name != "" {
			if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("catalog %q license name", catalog.ID), catalog.License.Name, false); err != nil {
				return err
			}
		}
		if catalog.License.URL != "" {
			if catalog.License.Name == "" {
				return fmt.Errorf("catalog %q license name is required with URL", catalog.ID)
			}
			if err := validatePublicHTTPSURL(fmt.Sprintf("catalog %q license URL", catalog.ID), catalog.License.URL); err != nil {
				return err
			}
		}
		if catalog.DefaultDocumentKey != "" {
			if err := domain.ValidateCatalogDocumentKey(catalog.DefaultDocumentKey); err != nil {
				return fmt.Errorf("catalog %q default document: %w", catalog.ID, err)
			}
		}
		if err := validateMount(catalog.Mount); err != nil {
			return fmt.Errorf("catalog %q mount: %w", catalog.ID, err)
		}
		if err := validateSEO(fmt.Sprintf("catalog %q", catalog.ID), catalog.SEO); err != nil {
			return err
		}
		configuredLocalDocs := catalog.LocalDocs.Public || catalog.LocalDocs.Anonymous || catalog.LocalDocs.PublicationKey != ""
		if configuredLocalDocs {
			if !catalog.LocalDocs.Public || !catalog.LocalDocs.Anonymous {
				return fmt.Errorf("catalog %q local docs requires public and anonymous authority", catalog.ID)
			}
			if err := domain.ValidateCatalogPublicationKey(catalog.LocalDocs.PublicationKey); err != nil {
				return fmt.Errorf("catalog %q local docs: %w", catalog.ID, err)
			}
			if owner, exists := publicationKeys[catalog.LocalDocs.PublicationKey]; exists {
				return fmt.Errorf("catalogs %q and %q duplicate local docs publication key %q", owner, catalog.ID, catalog.LocalDocs.PublicationKey)
			}
			publicationKeys[catalog.LocalDocs.PublicationKey] = catalog.ID
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

func validateOrganization(organization OrganizationConfig) error {
	if organization.Title != "" {
		if err := domain.ValidateCanonicalIdentity("organization title", organization.Title, false); err != nil {
			return err
		}
	}
	if len(organization.Readme) > maxOrganizationReadmeBytes {
		return fmt.Errorf("organization README exceeds %d bytes", maxOrganizationReadmeBytes)
	}
	if organization.Readme != "" {
		if err := domain.ValidateCanonicalIdentity("organization README", organization.Readme, false); err != nil {
			return err
		}
	}
	if organization.License.Name != "" {
		if err := domain.ValidateCanonicalIdentity("organization license name", organization.License.Name, false); err != nil {
			return err
		}
	}
	if organization.License.URL != "" {
		if organization.License.Name == "" {
			return fmt.Errorf("organization license name is required with URL")
		}
		if err := validatePublicHTTPSURL("organization license URL", organization.License.URL); err != nil {
			return err
		}
	}
	if len(organization.Sources) > maxOrganizationSources {
		return fmt.Errorf("organization sources exceed %d", maxOrganizationSources)
	}
	for index, source := range organization.Sources {
		prefix := fmt.Sprintf("organization source %d", index)
		for label, value := range map[string]string{"name": source.Name, "kind": source.Kind, "location": source.Location} {
			if err := domain.ValidateCanonicalIdentity(prefix+" "+label, value, false); err != nil {
				return err
			}
		}
		if source.Kind != OrganizationSourceKindGit && source.Kind != OrganizationSourceKindFile {
			return fmt.Errorf("%s kind %q is unsupported", prefix, source.Kind)
		}
		if source.URL != "" {
			if err := validatePublicHTTPSURL(prefix+" URL", source.URL); err != nil {
				return err
			}
		}
	}
	return validateSEO("organization", organization.SEO)
}

func validateSEO(owner string, seo CatalogSEO) error {
	for label, value := range map[string]string{"description": seo.Description, "social image alt": seo.SocialImageAlt} {
		if value != "" {
			if err := domain.ValidateCanonicalIdentity(fmt.Sprintf("%s SEO %s", owner, label), value, false); err != nil {
				return err
			}
		}
	}
	for label, value := range map[string]string{"canonical base": seo.CanonicalBase, "social image": seo.SocialImage} {
		if value == "" {
			continue
		}
		if err := validatePublicHTTPSURL(fmt.Sprintf("%s SEO %s", owner, label), value); err != nil {
			return err
		}
	}
	if seo.SocialImage != "" && seo.SocialImageAlt == "" {
		return fmt.Errorf("%s SEO social image alt is required with social image", owner)
	}
	if seo.SocialImage != "" && socialImageMIMEType(seo.SocialImage) == "" {
		return fmt.Errorf("%s SEO social image must use a supported .png, .jpg, .jpeg, or .webp extension", owner)
	}
	return nil
}

func validatePublicHTTPSURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials, query, or fragment", name)
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

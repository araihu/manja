package renderer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestNewAcceptsCanonicalRootAndNestedCatalogConfigurations(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]Config{
		"root": {
			Version:  1,
			Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict}},
		},
		"multiple": {
			Version: 1,
			Catalogs: []CatalogConfig{
				{ID: "kubernetes", Mount: "/kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileKubernetes},
				{ID: "payments", Mount: "/payments", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(config); err != nil {
				t.Fatalf("New: %v", err)
			}
		})
	}
}

func TestCatalogCountLimitIsOptIn(t *testing.T) {
	config := Config{Version: 1, Catalogs: repeatedCatalogConfigs(maxConfiguredCatalogs + 1)}
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("default configuration rejected experimental catalog count: %v", err)
	}
	config.ResourceLimits = true
	if err := ValidateConfig(config); err == nil {
		t.Fatal("resource-limited configuration accepted catalog count above the budget")
	}
}

func TestNewRejectsInvalidCatalogConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{name: "version", config: Config{Version: 2, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/")}}},
		{name: "catalog required", config: Config{Version: 1}},
		{name: "catalog maximum", config: Config{Version: 1, ResourceLimits: true, Catalogs: repeatedCatalogConfigs(9)}},
		{name: "duplicate id", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/payments"), validCatalogConfig("payments", "/other")}}},
		{name: "uppercase id", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("Payments", "/payments")}}},
		{name: "empty title", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/payments", ProfileID: domain.CompatibilityProfileStrict}}}},
		{name: "empty profile", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/payments", Title: "Payments"}}}},
		{name: "relative mount", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "payments")}}},
		{name: "trailing slash", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/payments/")}}},
		{name: "duplicate slash", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/docs//payments")}}},
		{name: "dot segment", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/docs/../payments")}}},
		{name: "encoded slash", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/docs%2fpayments")}}},
		{name: "backslash", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", `/docs\payments`)}}},
		{name: "query", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/payments?mode=full")}}},
		{name: "reserved assets", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/assets/payments")}}},
		{name: "reserved health", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/healthz")}}},
		{name: "duplicate mount", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/docs"), validCatalogConfig("other", "/docs")}}},
		{name: "overlap", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/docs"), validCatalogConfig("other", "/docs/other")}}},
		{name: "root overlap", config: Config{Version: 1, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/"), validCatalogConfig("other", "/other")}}},
		{name: "noncanonical default document", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, DefaultDocumentKey: "Core/V1"}}}},
		{name: "insecure canonical", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, SEO: CatalogSEO{CanonicalBase: "http://docs.example.test"}}}}},
		{name: "social image without alt", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, SEO: CatalogSEO{SocialImage: "https://docs.example.test/social.png"}}}}},
		{name: "social image query", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, SEO: CatalogSEO{SocialImage: "https://docs.example.test/social.png?v=1", SocialImageAlt: "Preview"}}}}},
		{name: "unsupported social image type", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, SEO: CatalogSEO{SocialImage: "https://docs.example.test/social.svg", SocialImageAlt: "Preview"}}}}},
		{name: "catalog license URL without name", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, License: CatalogLicense{URL: "https://example.test/license"}}}}},
		{name: "local docs public only", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, PublicationKey: "payments"}}}}},
		{name: "local docs anonymous only", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Anonymous: true, PublicationKey: "payments"}}}}},
		{name: "local docs key only", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{PublicationKey: "payments"}}}}},
		{name: "local docs missing key", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true}}}}},
		{name: "local docs uppercase key", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "Public-Payments"}}}}},
		{name: "local docs invalid key", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "public/payments"}}}}},
		{name: "local docs oversized key", config: Config{Version: 1, Catalogs: []CatalogConfig{{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: strings.Repeat("a", 65)}}}}},
		{name: "local docs duplicate eligible key", config: Config{Version: 1, Catalogs: []CatalogConfig{
			{ID: "payments", Mount: "/payments", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "public-api"}},
			{ID: "orders", Mount: "/orders", Title: "Orders", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "public-api"}},
		}}},
		{name: "organization source kind", config: Config{Version: 1, Organization: OrganizationConfig{Sources: []OrganizationSource{{Name: "API", Kind: "network", Location: "example"}}}, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/")}}},
		{name: "organization source URL", config: Config{Version: 1, Organization: OrganizationConfig{Sources: []OrganizationSource{{Name: "API", Kind: OrganizationSourceKindGit, Location: "example", URL: "http://example.test/repo"}}}, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/")}}},
		{name: "organization license URL without name", config: Config{Version: 1, Organization: OrganizationConfig{License: OrganizationLicense{URL: "https://example.test/license"}}, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.config); err == nil {
				t.Fatal("invalid renderer configuration was accepted")
			}
		})
	}
}

func TestNewAcceptsExplicitPublicAnonymousLocalDocsAuthority(t *testing.T) {
	t.Parallel()
	catalog := validCatalogConfig("payments", "/")
	catalog.LocalDocs = CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "public-payments"}
	if _, err := New(Config{Version: 1, Catalogs: []CatalogConfig{catalog}}); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewAcceptsDistinctCanonicalLocalDocsPublicationKeys(t *testing.T) {
	t.Parallel()
	config := Config{Version: 1, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/payments", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "public.payments-v1"}},
		{ID: "orders", Mount: "/orders", Title: "Orders", ProfileID: domain.CompatibilityProfileStrict, LocalDocs: CatalogLocalDocs{Public: true, Anonymous: true, PublicationKey: "public_orders-v1"}},
	}}
	if _, err := New(config); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewAcceptsOptionalOrganizationPresentation(t *testing.T) {
	t.Parallel()

	config := Config{
		Version: 1,
		Organization: OrganizationConfig{
			Title: "Manja", Readme: "OpenAPI workbench.",
			License: OrganizationLicense{Name: "Apache-2.0", URL: "https://example.test/license"},
			Sources: []OrganizationSource{{Name: "API definitions", Kind: OrganizationSourceKindGit, Location: "github.com/example/api", URL: "https://github.com/example/api"}},
			SEO:     CatalogSEO{Description: "OpenAPI workbench.", CanonicalBase: "https://example.test", SocialImage: "https://example.test/social.png", SocialImageAlt: "Preview"},
		},
		Catalogs: []CatalogConfig{validCatalogConfig("payments", "/")},
	}
	if _, err := New(config); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewAcceptsOptionalCatalogReadmeAndLicense(t *testing.T) {
	t.Parallel()

	catalog := validCatalogConfig("payments", "/")
	catalog.Readme = "Payments API documentation."
	catalog.License = CatalogLicense{Name: "Apache-2.0", URL: "https://example.test/license"}
	if _, err := New(Config{Version: 1, Catalogs: []CatalogConfig{catalog}}); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewAcceptsSupportedSocialImageTypes(t *testing.T) {
	t.Parallel()

	for _, extension := range []string{"png", "jpg", "jpeg", "webp"} {
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			catalog := validCatalogConfig("payments", "/")
			catalog.SEO = CatalogSEO{SocialImage: "https://docs.example.test/social." + extension, SocialImageAlt: "Preview"}
			if _, err := New(Config{Version: 1, Catalogs: []CatalogConfig{catalog}}); err != nil {
				t.Fatalf("New: %v", err)
			}
		})
	}
}

func TestPublicRendererConfigurationExposesNoCredentialFields(t *testing.T) {
	t.Parallel()

	seen := make(map[reflect.Type]bool)
	var visit func(reflect.Type)
	visit = func(value reflect.Type) {
		for value.Kind() == reflect.Pointer || value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct || seen[value] {
			return
		}
		seen[value] = true
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if !field.IsExported() {
				continue
			}
			normalized := strings.ToLower(field.Name)
			for _, forbidden := range []string{"password", "privatekey", "token", "secretvalue", "secretbytes"} {
				if strings.Contains(normalized, forbidden) {
					t.Errorf("public field %s.%s contains forbidden credential term %q", value, field.Name, forbidden)
				}
			}
			visit(field.Type)
		}
	}
	visit(reflect.TypeOf(Config{}))
}

func validCatalogConfig(id, mount string) CatalogConfig {
	return CatalogConfig{ID: id, Mount: mount, Title: id, ProfileID: domain.CompatibilityProfileStrict}
}

func repeatedCatalogConfigs(count int) []CatalogConfig {
	result := make([]CatalogConfig, count)
	for index := range result {
		id := "catalog-" + string(rune('a'+index))
		result[index] = validCatalogConfig(id, "/"+id)
	}
	return result
}

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

func TestNewRejectsInvalidCatalogConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{name: "version", config: Config{Version: 2, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/")}}},
		{name: "catalog required", config: Config{Version: 1}},
		{name: "catalog maximum", config: Config{Version: 1, Catalogs: repeatedCatalogConfigs(9)}},
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

package selfhosted

import (
	"context"
	"errors"
	"path"
	"strings"
	"unicode"
)

type ExportOptions struct {
	RendererOptions
	Output   string
	BasePath string
}

type ExportReceipt struct {
	SchemaVersion uint32                 `json:"schemaVersion"`
	BasePath      string                 `json:"basePath"`
	Catalogs      []ExportCatalogReceipt `json:"catalogs"`
	Manifest      string                 `json:"manifest"`
}

type ExportCatalogReceipt struct {
	CatalogID      string `json:"catalogId"`
	Mount          string `json:"mount"`
	PublicationKey string `json:"publicationKey"`
	RevisionID     string `json:"revisionId"`
	SnapshotID     string `json:"snapshotId"`
}

func ExportRenderer(context.Context, ExportOptions) (ExportReceipt, error) {
	return ExportReceipt{}, errors.New("static export is not implemented")
}

func VerifyExport(context.Context, string) (ExportReceipt, error) {
	return ExportReceipt{}, errors.New("static export verification is not implemented")
}

func canonicalExportBasePath(value string) error {
	if value == "/" {
		return nil
	}
	if len(value) < 3 || len(value) > 1024 || !strings.HasPrefix(value, "/") || !strings.HasSuffix(value, "/") || strings.Contains(value, `\`) || strings.ContainsAny(value, "%?#") {
		return errors.New("base path must be / or a canonical absolute path ending in /")
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return errors.New("base path must not contain whitespace or control characters")
		}
	}
	trimmed := strings.TrimSuffix(value, "/")
	if path.Clean(trimmed) != trimmed || strings.Contains(trimmed, "//") {
		return errors.New("base path must not contain duplicate slashes or dot segments")
	}
	return nil
}

package openapi

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/araihu/manja/domain"
)

func loadCapturedSpec(ctx context.Context, file domain.SpecFile, captured map[string][]byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.Context = ctx
	loader.ReadFromURIFunc = func(_ *openapi3.Loader, location *url.URL) ([]byte, error) {
		if location == nil || location.Scheme != "" || location.Host != "" || location.User != nil || location.Opaque != "" || location.RawQuery != "" || location.RawPath != "" {
			return nil, fmt.Errorf("catalog reference URI is forbidden")
		}
		sourcePath := location.Path
		if sourcePath == "" || strings.HasPrefix(sourcePath, "/") || strings.Contains(sourcePath, `\`) || strings.Contains(sourcePath, "%") || sourcePath == "." || path.Clean(sourcePath) != sourcePath || strings.HasPrefix(sourcePath, "../") {
			return nil, fmt.Errorf("catalog reference path %q is forbidden", sourcePath)
		}
		data, exists := captured[sourcePath]
		if !exists {
			return nil, fmt.Errorf("catalog reference path %q was not captured", sourcePath)
		}
		return append([]byte(nil), data...), nil
	}
	return loader.LoadFromDataWithPath(file.Bytes, &url.URL{Path: file.Path})
}

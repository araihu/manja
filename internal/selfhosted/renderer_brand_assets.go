package selfhosted

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type rendererBrandAsset struct {
	data        []byte
	contentType string
}
type rendererBrandAssets struct {
	next   http.Handler
	assets map[string]rendererBrandAsset
}

func withRendererBrandAssets(ctx context.Context, root string, next http.Handler) (http.Handler, error) {
	if root == "" {
		return next, nil
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("branding asset directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("branding asset directory must be a real directory")
	}
	handler := &rendererBrandAssets{next: next, assets: make(map[string]rendererBrandAsset)}
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("branding asset %q must not be a symlink", filename)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.ContainsAny(relative, "\\%?# \t\r\n") || strings.HasPrefix(relative, ".") {
			return fmt.Errorf("invalid branding asset path %q", relative)
		}
		extension := strings.ToLower(filepath.Ext(relative))
		switch extension {
		case ".svg", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico":
		default:
			return fmt.Errorf("branding asset %q must be an image", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 8<<20 {
			return fmt.Errorf("branding asset %q must be a regular image of at most 8 MiB", relative)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		handler.assets["/brand-assets/"+relative] = rendererBrandAsset{data: data, contentType: mime.TypeByExtension(extension)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return handler, nil
}

func (handler *rendererBrandAssets) AssetPaths() []string {
	paths := make([]string, 0, len(handler.assets))
	for path := range handler.assets {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (handler *rendererBrandAssets) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.URL.Path, "/brand-assets/") {
		handler.next.ServeHTTP(response, request)
		return
	}
	asset, ok := handler.assets[request.URL.Path]
	if !ok {
		http.NotFound(response, request)
		return
	}
	response.Header().Set("Content-Type", asset.contentType)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	response.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(response, request, filepath.Base(request.URL.Path), time.Time{}, bytes.NewReader(asset.data))
}

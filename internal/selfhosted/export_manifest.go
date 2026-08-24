package selfhosted

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type exportManifest struct {
	SchemaVersion uint32                 `json:"schemaVersion"`
	BasePath      string                 `json:"basePath"`
	Catalogs      []ExportCatalogReceipt `json:"catalogs"`
	Files         []exportFileEntry      `json:"files"`
}

type exportFileEntry struct {
	Path      string `json:"path"`
	Length    uint64 `json:"length"`
	MediaType string `json:"mediaType"`
	SHA256    string `json:"sha256"`
}

func encodeExportManifest(manifest exportManifest) ([]byte, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode export manifest: %w", err)
	}
	return data, nil
}

func VerifyExport(ctx context.Context, output string) (ExportReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	root, err := filepath.Abs(output)
	if err != nil {
		return ExportReceipt{}, fmt.Errorf("resolve export: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(exportManifestPath)))
	if err != nil {
		return ExportReceipt{}, fmt.Errorf("read export manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest exportManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ExportReceipt{}, fmt.Errorf("decode export manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ExportReceipt{}, err
	}
	canonical, err := encodeExportManifest(manifest)
	if err != nil || !bytes.Equal(data, canonical) {
		return ExportReceipt{}, errors.New("export manifest is not canonical")
	}
	if manifest.SchemaVersion != 1 || canonicalExportBasePath(manifest.BasePath) != nil {
		return ExportReceipt{}, errors.New("export manifest identity is invalid")
	}
	declared := make(map[string]exportFileEntry, len(manifest.Files)+1)
	previous := ""
	for _, entry := range manifest.Files {
		if entry.Path == exportManifestPath || entry.Path <= previous || entry.MediaType == "" || len(entry.SHA256) != sha256.Size*2 {
			return ExportReceipt{}, errors.New("export manifest file inventory is invalid")
		}
		previous = entry.Path
		declared[entry.Path] = entry
	}
	actual := make(map[string]struct{}, len(declared)+1)
	err = filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == root {
			return nil
		}
		info, err := os.Lstat(filename)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("export contains a symlink")
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("export contains a non-regular file")
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		actual[relative] = struct{}{}
		if relative == exportManifestPath {
			return nil
		}
		want, ok := declared[relative]
		if !ok {
			return fmt.Errorf("undeclared export file %q", relative)
		}
		contents, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(contents)
		if uint64(len(contents)) != want.Length || hex.EncodeToString(digest[:]) != want.SHA256 {
			return fmt.Errorf("export file %q differs from manifest", relative)
		}
		return nil
	})
	if err != nil {
		return ExportReceipt{}, err
	}
	if len(actual) != len(declared)+1 {
		return ExportReceipt{}, errors.New("export file set differs from manifest")
	}
	for name := range declared {
		if _, ok := actual[name]; !ok {
			return ExportReceipt{}, fmt.Errorf("missing export file %q", name)
		}
	}
	if _, ok := actual[exportManifestPath]; !ok {
		return ExportReceipt{}, errors.New("export manifest is missing")
	}
	return exportReceipt(manifest), nil
}

func exportReceipt(manifest exportManifest) ExportReceipt {
	catalogs := append([]ExportCatalogReceipt(nil), manifest.Catalogs...)
	return ExportReceipt{SchemaVersion: manifest.SchemaVersion, BasePath: manifest.BasePath, Catalogs: catalogs, Manifest: exportManifestPath}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("export manifest must contain exactly one JSON value")
	}
	return nil
}

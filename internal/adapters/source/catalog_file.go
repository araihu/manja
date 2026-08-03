package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type FileCatalogSource struct {
	Root           string
	Manifest       CatalogManifest
	StabilityDelay time.Duration

	afterFirstPass func()
}

func (source FileCatalogSource) Load(ctx context.Context) (domain.CatalogCandidate, error) {
	if err := ctx.Err(); err != nil {
		return domain.CatalogCandidate{}, err
	}
	root, err := canonicalCatalogRoot(source.Root)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	first, err := source.loadPass(ctx, root)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	if source.afterFirstPass != nil {
		source.afterFirstPass()
	}
	if source.StabilityDelay > 0 {
		timer := time.NewTimer(source.StabilityDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return domain.CatalogCandidate{}, ctx.Err()
		case <-timer.C:
		}
	}
	second, err := source.loadPass(ctx, root)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	if first.Revision.ManifestDigest != second.Revision.ManifestDigest {
		return domain.CatalogCandidate{}, fmt.Errorf("file catalog changed between stable reads")
	}
	return second, nil
}

func (source FileCatalogSource) loadPass(ctx context.Context, root string) (domain.CatalogCandidate, error) {
	inventory, err := fileCatalogInventory(root)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	reader := func(ctx context.Context, entry catalogInventoryEntry) (capturedCatalogFile, error) {
		if err := ctx.Err(); err != nil {
			return capturedCatalogFile{}, err
		}
		return readCatalogFile(root, entry.path)
	}
	return captureCatalogCandidate(ctx, source.Manifest, inventory, reader, domain.CatalogRevisionFiles, "")
}

func canonicalCatalogRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("file catalog root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve file catalog root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve file catalog root: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("stat file catalog root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("file catalog root is not a directory")
	}
	return canonical, nil
}

func fileCatalogInventory(root string) ([]catalogInventoryEntry, error) {
	var result []catalogInventoryEntry
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == root {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		sourcePath := filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			canonical, err := filepath.EvalSymlinks(filename)
			if err != nil {
				return fmt.Errorf("resolve catalog symlink %q: %w", sourcePath, err)
			}
			if !pathInsideCatalogRoot(root, canonical) {
				return fmt.Errorf("catalog symlink %q escapes the source root", sourcePath)
			}
			info, err := os.Stat(canonical)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("catalog directory symlink %q is unsupported", sourcePath)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("catalog path %q is not a regular file", sourcePath)
			}
			result = append(result, catalogInventoryEntry{path: sourcePath, mode: fileModeIdentity(info.Mode())})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("catalog path %q is not a regular file", sourcePath)
		}
		result = append(result, catalogInventoryEntry{path: sourcePath, mode: fileModeIdentity(info.Mode())})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk file catalog: %w", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result, nil
}

func readCatalogFile(root, sourcePath string) (capturedCatalogFile, error) {
	filename := filepath.Join(root, filepath.FromSlash(sourcePath))
	canonical, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	if !pathInsideCatalogRoot(root, canonical) {
		return capturedCatalogFile{}, fmt.Errorf("catalog path %q escapes the source root", sourcePath)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	if !info.Mode().IsRegular() {
		return capturedCatalogFile{}, fmt.Errorf("catalog path %q is not a regular file", sourcePath)
	}
	if info.Size() > maxCatalogSourceFileBytes {
		return capturedCatalogFile{}, fmt.Errorf("captured file %q exceeds %d bytes", sourcePath, maxCatalogSourceFileBytes)
	}
	data, err := os.ReadFile(canonical)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	return capturedCatalogFile{path: sourcePath, mode: fileModeIdentity(info.Mode()), data: data}, nil
}

func pathInsideCatalogRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func fileModeIdentity(mode os.FileMode) string {
	return fmt.Sprintf("%04o", mode.Perm())
}

var _ port.CatalogSource = FileCatalogSource{}

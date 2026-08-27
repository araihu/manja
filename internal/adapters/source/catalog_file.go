package source

import (
	"context"
	"fmt"
	"io"
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
	beforeFileRead func(string)
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
	resourceLimits := !source.Manifest.DisableResourceLimits
	inventory, err := fileCatalogInventory(ctx, root, resourceLimits)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	reader := func(ctx context.Context, entry catalogInventoryEntry) (capturedCatalogFile, error) {
		if err := ctx.Err(); err != nil {
			return capturedCatalogFile{}, err
		}
		return readCatalogFile(root, entry, source.beforeFileRead, resourceLimits)
	}
	sizer := func(ctx context.Context, entry catalogInventoryEntry) (int64, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return entry.size, nil
	}
	return captureCatalogCandidate(ctx, source.Manifest, inventory, sizer, reader, domain.CatalogRevisionFiles, "")
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

func fileCatalogInventory(ctx context.Context, root string, resourceLimits bool) ([]catalogInventoryEntry, error) {
	var result []catalogInventoryEntry
	var pathBytes int
	var sourceBytes int64
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
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
			return appendFileCatalogInventoryEntry(&result, &pathBytes, &sourceBytes, sourcePath, info, resourceLimits)
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
		return appendFileCatalogInventoryEntry(&result, &pathBytes, &sourceBytes, sourcePath, info, resourceLimits)
	})
	if err != nil {
		return nil, fmt.Errorf("walk file catalog: %w", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result, nil
}

func appendFileCatalogInventoryEntry(result *[]catalogInventoryEntry, pathBytes *int, sourceBytes *int64, sourcePath string, info os.FileInfo, resourceLimits bool) error {
	if resourceLimits && len(*result) >= maxCatalogInventoryEntries {
		return fmt.Errorf("catalog inventory exceeds %d entries", maxCatalogInventoryEntries)
	}
	*pathBytes += len(sourcePath)
	if resourceLimits && *pathBytes > maxCatalogInventoryBytes {
		return fmt.Errorf("catalog inventory paths exceed %d bytes", maxCatalogInventoryBytes)
	}
	if info.Size() < 0 || resourceLimits && info.Size() > maxCatalogSourceFileBytes {
		return fmt.Errorf("catalog inventory file %q exceeds %d bytes", sourcePath, maxCatalogSourceFileBytes)
	}
	*sourceBytes += info.Size()
	if resourceLimits && *sourceBytes > maxCatalogSourceBytes {
		return fmt.Errorf("catalog inventory source bytes exceed %d", maxCatalogSourceBytes)
	}
	*result = append(*result, catalogInventoryEntry{path: sourcePath, mode: fileModeIdentity(info.Mode()), size: info.Size()})
	return nil
}

func readCatalogFile(root string, entry catalogInventoryEntry, beforeRead func(string), resourceLimits bool) (capturedCatalogFile, error) {
	sourcePath := entry.path
	filename := filepath.Join(root, filepath.FromSlash(sourcePath))
	canonical, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	if !pathInsideCatalogRoot(root, canonical) {
		return capturedCatalogFile{}, fmt.Errorf("catalog path %q escapes the source root", sourcePath)
	}
	pathInfo, err := os.Lstat(canonical)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return capturedCatalogFile{}, fmt.Errorf("catalog path %q is not a regular file", sourcePath)
	}
	file, err := os.Open(canonical)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return capturedCatalogFile{}, err
	}
	if !os.SameFile(pathInfo, info) || info.Size() != entry.size || fileModeIdentity(info.Mode()) != entry.mode {
		return capturedCatalogFile{}, fmt.Errorf("catalog file %q changed before reading", sourcePath)
	}
	if info.Size() < 0 || resourceLimits && info.Size() > maxCatalogSourceFileBytes {
		return capturedCatalogFile{}, fmt.Errorf("captured file %q exceeds %d bytes", sourcePath, maxCatalogSourceFileBytes)
	}
	if beforeRead != nil {
		beforeRead(sourcePath)
	}
	var reader io.Reader = file
	if resourceLimits {
		reader = io.LimitReader(file, maxCatalogSourceFileBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return capturedCatalogFile{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return capturedCatalogFile{}, err
	}
	if resourceLimits && len(data) > maxCatalogSourceFileBytes {
		return capturedCatalogFile{}, fmt.Errorf("captured file %q exceeds %d bytes", sourcePath, maxCatalogSourceFileBytes)
	}
	if int64(len(data)) != info.Size() || after.Size() != info.Size() || after.ModTime() != info.ModTime() {
		return capturedCatalogFile{}, fmt.Errorf("catalog file %q changed while reading", sourcePath)
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

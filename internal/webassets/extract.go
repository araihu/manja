package webassets

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/araihu/assets/assetmeta"
)

const (
	maxArchiveFileSize = 8 << 20
	maxArchiveTotal    = 64 << 20
)

type archiveOpener func(assetmeta.Ref) (io.ReadCloser, error)

func Stage(ctx context.Context, root string, packages []Package) (string, error) {
	return stageWithOpener(ctx, root, packages, func(ref assetmeta.Ref) (io.ReadCloser, error) {
		return MuambaOpen(ref.Resource, ref.Download)
	})
}

func stageWithOpener(ctx context.Context, root string, packages []Package, opener archiveOpener) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if opener == nil {
		return "", fmt.Errorf("archive opener is nil")
	}
	seen := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		if !validPackageName(pkg.Name) {
			return "", fmt.Errorf("invalid package name %q", pkg.Name)
		}
		if _, exists := seen[pkg.Name]; exists {
			return "", fmt.Errorf("duplicate package name %q", pkg.Name)
		}
		seen[pkg.Name] = struct{}{}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	stageRoot, err := os.MkdirTemp(root, ".webassets-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stageRoot)
	modules := filepath.Join(stageRoot, "node_modules")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		return "", err
	}
	for _, pkg := range packages {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		archive, err := opener(pkg.ArchiveRef)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", pkg.Resource, err)
		}
		destination := filepath.Join(modules, filepath.FromSlash(pkg.Name))
		extractErr := extractArchive(ctx, archive, destination)
		closeErr := archive.Close()
		if extractErr != nil || closeErr != nil {
			return "", errors.Join(fmt.Errorf("extract %s: %w", pkg.Resource, extractErr), closeErr)
		}
	}
	final := filepath.Join(root, "node_modules")
	if err := publishDirectory(modules, final); err != nil {
		return "", err
	}
	return final, nil
}

func extractArchive(ctx context.Context, source io.Reader, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	gz, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	seen := make(map[string]struct{})
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if header.Name == "package" || header.Name == "package/" {
			if header.Typeflag != tar.TypeDir {
				return fmt.Errorf("package root is not a directory")
			}
			continue
		}
		relative, ok := strings.CutPrefix(header.Name, "package/")
		if !ok || !fs.ValidPath(relative) || strings.Contains(relative, `\`) || path.Clean(relative) != relative {
			return fmt.Errorf("unsafe archive member %q", header.Name)
		}
		if _, duplicate := seen[relative]; duplicate {
			return fmt.Errorf("duplicate archive member %q", relative)
		}
		seen[relative] = struct{}{}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(relative, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxArchiveFileSize {
				return fmt.Errorf("file size %d exceeds limit for %q", header.Size, relative)
			}
			total += header.Size
			if total > maxArchiveTotal {
				return fmt.Errorf("archive expansion exceeds %d bytes", maxArchiveTotal)
			}
			if err := root.MkdirAll(path.Dir(relative), 0o755); err != nil {
				return err
			}
			file, err := root.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			copyErr := copyContext(ctx, file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
		default:
			return fmt.Errorf("unsupported archive member type %d for %q", header.Typeflag, header.Name)
		}
	}
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader, size int64) error {
	remaining := size
	buffer := make([]byte, 32*1024)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := int64(len(buffer))
		if remaining < chunk {
			chunk = remaining
		}
		read, err := io.ReadFull(source, buffer[:chunk])
		if err != nil {
			return err
		}
		if _, err := destination.Write(buffer[:read]); err != nil {
			return err
		}
		remaining -= int64(read)
	}
	return nil
}

func publishDirectory(staged, destination string) error {
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		return os.Rename(staged, destination)
	} else if err != nil {
		return err
	}
	backupRoot, err := os.MkdirTemp(filepath.Dir(destination), ".webassets-backup-")
	if err != nil {
		return err
	}
	if err := os.Remove(backupRoot); err != nil {
		return err
	}
	if err := os.Rename(destination, backupRoot); err != nil {
		return err
	}
	if err := os.Rename(staged, destination); err != nil {
		return errors.Join(err, os.Rename(backupRoot, destination))
	}
	return os.RemoveAll(backupRoot)
}

func validPackageName(name string) bool {
	if name == "" || strings.ContainsAny(name, `\:`) {
		return false
	}
	parts := strings.Split(name, "/")
	if strings.HasPrefix(name, "@") {
		return len(parts) == 2 && validPackageSegment(parts[0][1:]) && validPackageSegment(parts[1])
	}
	return len(parts) == 1 && validPackageSegment(parts[0])
}

func validPackageSegment(value string) bool {
	return value != "" && value != "." && value != ".." && fs.ValidPath(value)
}

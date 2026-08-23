package catalogstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/adapters/catalogjson"
)

func (store *Store) Preflight(ctx context.Context, id catalog.SnapshotID) (catalog.RuntimeSnapshot, error) {
	location, err := store.snapshotPath(id)
	if err != nil {
		return catalog.RuntimeSnapshot{}, err
	}
	manifestLimit := int64(0)
	if store.resourceLimits {
		manifestLimit = 4 << 20
	}
	manifestBytes, err := readBounded(filepath.Join(location, "manifest.json"), manifestLimit)
	if err != nil {
		return catalog.RuntimeSnapshot{}, classifyRead("manifest.json", err)
	}
	manifest, err := catalogjson.DecodeManifestWithResourceLimits(manifestBytes, store.resourceLimits)
	if err != nil || manifest.SnapshotID != id {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: decode manifest: %v", ErrCorruptSnapshot, err)
	}
	children := make(map[string]catalog.ChildIdentityV1, len(manifest.Children))
	for _, child := range manifest.Children {
		if err := ctx.Err(); err != nil {
			return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: %v", ErrTransientSnapshot, err)
		}
		childPath, pathErr := safeChildPath(location, child.Path)
		if pathErr != nil {
			return catalog.RuntimeSnapshot{}, pathErr
		}
		if err := verifyFile(childPath, child.Length, child.SHA256); err != nil {
			return catalog.RuntimeSnapshot{}, err
		}
		children[child.Path] = child
	}
	catalogIdentity, ok := children["catalog.json"]
	if !ok {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: catalog.json is undeclared", ErrCorruptSnapshot)
	}
	catalogBytes, err := readBounded(filepath.Join(location, "catalog.json"), int64(catalogIdentity.Length))
	if err != nil {
		return catalog.RuntimeSnapshot{}, classifyRead("catalog.json", err)
	}
	directory, err := catalogjson.DecodeCatalogWithResourceLimits(catalogBytes, store.resourceLimits)
	if err != nil {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: decode catalog: %v", ErrCorruptSnapshot, err)
	}
	if err := catalogjson.ValidateCatalogManifest(directory, manifest); err != nil {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: catalog references: %v", ErrCorruptSnapshot, err)
	}
	searchIdentity, ok := children[directory.SearchChild]
	if !ok {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: search directory is undeclared", ErrCorruptSnapshot)
	}
	searchBytes, err := readBounded(filepath.Join(location, filepath.FromSlash(directory.SearchChild)), int64(searchIdentity.Length))
	if err != nil {
		return catalog.RuntimeSnapshot{}, classifyRead(directory.SearchChild, err)
	}
	search, err := catalogjson.DecodeSearchDirectoryWithResourceLimits(searchBytes, store.resourceLimits)
	if err != nil {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: decode search directory: %v", ErrCorruptSnapshot, err)
	}
	if err := catalogjson.ValidateSearchManifest(search, manifest); err != nil {
		return catalog.RuntimeSnapshot{}, fmt.Errorf("%w: search references: %v", ErrCorruptSnapshot, err)
	}
	return catalog.RuntimeSnapshot{ID: id, Location: location, Directory: directory, Search: search, Manifest: manifest}, nil
}

func (store *Store) ReadChild(ctx context.Context, snapshot catalog.RuntimeSnapshot, childPath string) ([]byte, catalog.ChildIdentityV1, error) {
	if err := ctx.Err(); err != nil {
		return nil, catalog.ChildIdentityV1{}, err
	}
	var identity catalog.ChildIdentityV1
	found := false
	for _, child := range snapshot.Manifest.Children {
		if child.Path == childPath {
			identity = child
			found = true
			break
		}
	}
	if !found {
		return nil, catalog.ChildIdentityV1{}, fmt.Errorf("%w: child %q is undeclared", ErrCorruptSnapshot, childPath)
	}
	path, err := safeChildPath(snapshot.Location, childPath)
	if err != nil {
		return nil, catalog.ChildIdentityV1{}, err
	}
	data, err := readBounded(path, int64(identity.Length))
	if err != nil {
		return nil, catalog.ChildIdentityV1{}, classifyRead(childPath, err)
	}
	digest := sha256.Sum256(data)
	if uint64(len(data)) != identity.Length || hex.EncodeToString(digest[:]) != identity.SHA256 {
		return nil, catalog.ChildIdentityV1{}, fmt.Errorf("%w: child %q length or digest changed", ErrCorruptSnapshot, childPath)
	}
	return data, identity, nil
}

func verifyFile(path string, length uint64, digest string) error {
	file, err := os.Open(path)
	if err != nil {
		return classifyRead(path, err)
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, int64(length)+1))
	if err != nil {
		return fmt.Errorf("%w: stream %q: %v", ErrTransientSnapshot, path, err)
	}
	if uint64(written) != length || hex.EncodeToString(hash.Sum(nil)) != digest {
		return fmt.Errorf("%w: child %q length or digest changed", ErrCorruptSnapshot, path)
	}
	return nil
}

func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.Reader = file
	if maximum > 0 {
		reader = io.LimitReader(file, maximum+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maximum > 0 && int64(len(data)) > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes", maximum)
	}
	return data, nil
}

func classifyRead(path string, err error) error {
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: required child %q is missing", ErrCorruptSnapshot, path)
	}
	return fmt.Errorf("%w: read %q: %v", ErrTransientSnapshot, path, err)
}

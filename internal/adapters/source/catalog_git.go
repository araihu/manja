package source

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type GitCatalogSource struct {
	Repository    string
	Ref           string
	Root          string
	Username      string
	Token         string
	SSHPrivateKey string
	Manifest      CatalogManifest
	// IntegrityReceiptRoot and IntegrityReceiptPath identify an optional receipt
	// beneath a trusted root. Receipt paths never follow symlinks.
	IntegrityReceiptRoot string
	IntegrityReceiptPath string

	afterResolve func(string)
}

func (source GitCatalogSource) Load(ctx context.Context) (domain.CatalogCandidate, error) {
	if err := ctx.Err(); err != nil {
		return domain.CatalogCandidate{}, err
	}
	if strings.TrimSpace(source.Repository) == "" {
		return domain.CatalogCandidate{}, fmt.Errorf("Git catalog repository is required")
	}
	root := source.Root
	if root == "" {
		root = "."
	}
	if root != "." {
		if err := validateSourcePath("Git catalog root", root); err != nil {
			return domain.CatalogCandidate{}, err
		}
	}
	reference := source.Ref
	if reference == "" {
		reference = "HEAD"
	}
	var integrity *gitCatalogIntegrity
	if source.IntegrityReceiptRoot != "" || source.IntegrityReceiptPath != "" {
		receipt, err := loadGitSourceProvenanceReceipt(source.IntegrityReceiptRoot, source.IntegrityReceiptPath)
		if err != nil {
			return domain.CatalogCandidate{}, err
		}
		integrity, err = newGitCatalogIntegrity(receipt, source, root, reference)
		if err != nil {
			return domain.CatalogCandidate{}, err
		}
	}
	gitSource := Git{
		Repo: source.Repository, Username: source.Username, Token: source.Token, SSHPrivateKey: source.SSHPrivateKey,
	}
	remoteCatalog := true
	if info, statErr := os.Stat(gitSource.cloneURL()); statErr == nil && info.IsDir() {
		remoteCatalog = false
	}
	objectFormat := gitObjectFormat("")
	if integrity != nil {
		objectFormat = integrity.receipt.ObjectFormat
	}
	repository, resolvedRef, cleanup, err := gitCatalogRepositoryWithObjectFormat(ctx, gitSource.cloneURL(), reference, source.SSHPrivateKey, objectFormat)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	defer cleanup()
	if integrity != nil {
		resolvedObjectFormat, err := gitOutputLimit(ctx, repository, 16, "rev-parse", "--show-object-format=storage")
		if err != nil {
			return domain.CatalogCandidate{}, fmt.Errorf("inspect Git catalog object format: %w", err)
		}
		if err := integrity.verifyObjectFormat(resolvedObjectFormat); err != nil {
			return domain.CatalogCandidate{}, err
		}
	}
	commit, err := gitOutputLimit(ctx, repository, 128, "rev-parse", "--verify", resolvedRef+"^{commit}")
	if err != nil {
		return domain.CatalogCandidate{}, fmt.Errorf("resolve Git catalog ref %q: %w", reference, err)
	}
	if !isFullGitObjectID(commit) {
		return domain.CatalogCandidate{}, fmt.Errorf("resolved Git catalog ref %q is not a full commit object ID", reference)
	}
	if integrity != nil {
		tree, err := gitOutputLimit(ctx, repository, 128, "rev-parse", "--verify", commit+"^{tree}")
		if err != nil {
			return domain.CatalogCandidate{}, fmt.Errorf("resolve Git catalog tree at %q: %w", commit, err)
		}
		if !isFullGitObjectID(tree) {
			return domain.CatalogCandidate{}, fmt.Errorf("resolved Git catalog tree at %q is not a full object ID", commit)
		}
		if err := integrity.verifyRepository(commit, tree); err != nil {
			return domain.CatalogCandidate{}, err
		}
	}
	if source.afterResolve != nil {
		source.afterResolve(commit)
	}
	inventory, err := gitCatalogInventory(ctx, repository, commit, root)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	missingObjects := map[string]struct{}{}
	if remoteCatalog {
		missingObjects, err = gitCatalogMissingObjects(ctx, repository, commit, root)
		if err != nil {
			return domain.CatalogCandidate{}, err
		}
	}
	sizer := func(ctx context.Context, entry catalogInventoryEntry) (int64, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		objectPath := entry.path
		if root != "." {
			objectPath = path.Join(root, entry.path)
		}
		if entry.objectID == "" {
			return 0, fmt.Errorf("Git catalog object ID is missing for %q", entry.path)
		}
		if integrity != nil {
			if _, err := integrity.verifyMetadata(entry, -1); err != nil {
				return 0, err
			}
		}
		if _, missing := missingObjects[entry.objectID]; missing {
			return 0, fmt.Errorf("captured file %q exceeds %d bytes", entry.path, maxCatalogSourceFileBytes)
		}
		sizeBytes, err := gitOutputBytesEnvLimit(ctx, repository, []string{"GIT_NO_LAZY_FETCH=1"}, 32, "cat-file", "-s", entry.objectID)
		if err != nil {
			return 0, err
		}
		sizeText := strings.TrimSpace(string(sizeBytes))
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil || size < 0 {
			return 0, fmt.Errorf("read Git catalog object size for %q", objectPath)
		}
		if size > maxCatalogSourceFileBytes {
			return 0, fmt.Errorf("captured file %q exceeds %d bytes", entry.path, maxCatalogSourceFileBytes)
		}
		if integrity != nil {
			if _, err := integrity.verifyMetadata(entry, size); err != nil {
				return 0, err
			}
		}
		return size, nil
	}
	reader := func(ctx context.Context, entry catalogInventoryEntry) (capturedCatalogFile, error) {
		if err := ctx.Err(); err != nil {
			return capturedCatalogFile{}, err
		}
		data, err := gitOutputBytesEnvLimit(ctx, repository, []string{"GIT_NO_LAZY_FETCH=1"}, uint64(entry.size)+1, "cat-file", "blob", entry.objectID)
		if err != nil {
			return capturedCatalogFile{}, err
		}
		if int64(len(data)) != entry.size {
			return capturedCatalogFile{}, fmt.Errorf("Git catalog object %q changed length", entry.path)
		}
		if integrity != nil {
			if err := integrity.verifyBytes(entry, data); err != nil {
				return capturedCatalogFile{}, err
			}
		}
		return capturedCatalogFile{path: entry.path, mode: entry.mode, data: data}, nil
	}
	candidate, err := captureCatalogCandidate(ctx, source.Manifest, inventory, sizer, reader, domain.CatalogRevisionGit, commit)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	if integrity != nil {
		if err := integrity.verifyComplete(); err != nil {
			return domain.CatalogCandidate{}, err
		}
	}
	return candidate, nil
}

func gitCatalogMissingObjects(ctx context.Context, repository, commit, root string) (map[string]struct{}, error) {
	args := []string{"rev-list", "--objects", "--missing=print", commit}
	if root != "." {
		args = append(args, "--", root)
	}
	output, err := gitOutputBytesEnvLimit(ctx, repository, []string{"GIT_NO_LAZY_FETCH=1"}, maxCatalogInventoryBytes, args...)
	if err != nil {
		return nil, fmt.Errorf("inspect missing Git catalog objects at %s: %w", commit, err)
	}
	missing := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		objectID, exists := strings.CutPrefix(strings.TrimSpace(line), "?")
		if !exists {
			continue
		}
		if !isFullGitObjectID(objectID) {
			return nil, fmt.Errorf("invalid missing Git catalog object ID %q", objectID)
		}
		missing[objectID] = struct{}{}
	}
	return missing, nil
}

func gitCatalogInventory(ctx context.Context, repository, commit, root string) ([]catalogInventoryEntry, error) {
	args := []string{"ls-tree", "-rz", "--full-tree", commit}
	if root != "." {
		args = append(args, "--", root)
	}
	output, err := gitOutputBytesLimit(ctx, repository, maxCatalogInventoryBytes, args...)
	if err != nil {
		return nil, fmt.Errorf("list Git catalog tree at %s: %w", commit, err)
	}
	records := bytes.Split(output, []byte{0})
	result := make([]catalogInventoryEntry, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		metadata, objectPathBytes, exists := bytes.Cut(record, []byte{'\t'})
		if !exists {
			return nil, fmt.Errorf("parse Git catalog tree record")
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		if fields[0] == "120000" {
			return nil, fmt.Errorf("Git catalog tree contains symlink %q", objectPathBytes)
		}
		objectPath := string(objectPathBytes)
		sourcePath := objectPath
		if root != "." {
			prefix := strings.TrimSuffix(root, "/") + "/"
			if !strings.HasPrefix(objectPath, prefix) {
				continue
			}
			sourcePath = strings.TrimPrefix(objectPath, prefix)
		}
		if err := validateSourcePath("Git catalog tree path", sourcePath); err != nil {
			return nil, err
		}
		if len(result) >= maxCatalogInventoryEntries {
			return nil, fmt.Errorf("Git catalog inventory exceeds %d entries", maxCatalogInventoryEntries)
		}
		result = append(result, catalogInventoryEntry{path: sourcePath, mode: fields[0], objectID: fields[2]})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result, nil
}

func isFullGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

var _ port.CatalogSource = GitCatalogSource{}

package source

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/araihu/manja/domain"
)

const (
	maxCatalogSourceBytes      = 64 << 20
	maxKubernetesSourceBytes   = 16 << 20
	maxCatalogSourceFileBytes  = 8 << 20
	maxCatalogInventoryEntries = 1024
	maxCatalogInventoryBytes   = 2 << 20
)

type CatalogManifest struct {
	ID                 string
	Title              string
	Branding           domain.DocsBranding
	DefaultDocumentKey string
	ProfileID          domain.CompatibilityProfileID
	Includes           []string
	DocumentKeys       []CatalogDocumentKey
}

type CatalogDocumentKey struct {
	SourcePath string
	Key        string
}

type catalogInventoryEntry struct {
	path     string
	mode     string
	size     int64
	objectID string
}

type capturedCatalogFile struct {
	path string
	mode string
	data []byte
}

type catalogFileReader func(context.Context, catalogInventoryEntry) (capturedCatalogFile, error)
type catalogFileSizer func(context.Context, catalogInventoryEntry) (int64, error)

type catalogSourceBudget struct {
	limit int64
	used  int64
}

func (budget *catalogSourceBudget) reserve(size int64) error {
	if size <= 0 {
		return fmt.Errorf("catalog source file is empty")
	}
	if size > maxCatalogSourceFileBytes {
		return fmt.Errorf("catalog source file exceeds %d bytes", maxCatalogSourceFileBytes)
	}
	if size > budget.limit-budget.used {
		return fmt.Errorf("catalog source bytes %d exceed %d", budget.used+size, budget.limit)
	}
	budget.used += size
	return nil
}

func captureCatalogCandidate(
	ctx context.Context,
	manifest CatalogManifest,
	inventory []catalogInventoryEntry,
	size catalogFileSizer,
	read catalogFileReader,
	revisionKind domain.CatalogRevisionKind,
	commit string,
) (domain.CatalogCandidate, error) {
	if err := ctx.Err(); err != nil {
		return domain.CatalogCandidate{}, err
	}
	if err := validateCatalogManifest(manifest); err != nil {
		return domain.CatalogCandidate{}, err
	}
	inventory = append([]catalogInventoryEntry(nil), inventory...)
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].path < inventory[j].path })
	byPath := make(map[string]catalogInventoryEntry, len(inventory))
	for _, entry := range inventory {
		if err := validateSourcePath("catalog inventory path", entry.path); err != nil {
			return domain.CatalogCandidate{}, err
		}
		if entry.mode == "" {
			return domain.CatalogCandidate{}, fmt.Errorf("catalog inventory mode is required for %q", entry.path)
		}
		if _, exists := byPath[entry.path]; exists {
			return domain.CatalogCandidate{}, fmt.Errorf("catalog inventory path %q is duplicated", entry.path)
		}
		byPath[entry.path] = entry
	}

	selected, err := selectCatalogDocuments(manifest.Includes, inventory)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	keyByPath := make(map[string]string, len(manifest.DocumentKeys))
	for _, mapping := range manifest.DocumentKeys {
		keyByPath[mapping.SourcePath] = mapping.Key
	}
	documents := make([]domain.CatalogDocument, 0, len(selected))
	captured := make(map[string]capturedCatalogFile, len(selected))
	limit := int64(maxCatalogSourceBytes)
	if manifest.ProfileID == domain.CompatibilityProfileKubernetes {
		limit = maxKubernetesSourceBytes
	}
	budget := &catalogSourceBudget{limit: limit}
	for _, entry := range selected {
		verifiedSize, err := size(ctx, entry)
		if err != nil {
			return domain.CatalogCandidate{}, fmt.Errorf("size catalog document %q: %w", entry.path, err)
		}
		if err := budget.reserve(verifiedSize); err != nil {
			return domain.CatalogCandidate{}, fmt.Errorf("admit catalog document %q: %w", entry.path, err)
		}
		entry.size = verifiedSize
		file, err := read(ctx, entry)
		if err != nil {
			return domain.CatalogCandidate{}, fmt.Errorf("read catalog document %q: %w", entry.path, err)
		}
		if err := validateCapturedFile(file, entry); err != nil {
			return domain.CatalogCandidate{}, err
		}
		key := keyByPath[entry.path]
		if key == "" {
			key, err = catalogDocumentKey(entry.path)
			if err != nil {
				return domain.CatalogCandidate{}, err
			}
		}
		documents = append(documents, domain.CatalogDocument{
			Key: key, SourcePath: entry.path, Format: catalogFormat(entry.path), Bytes: file.data,
		})
		captured[entry.path] = file
	}
	for sourcePath := range keyByPath {
		if _, exists := captured[sourcePath]; !exists {
			return domain.CatalogCandidate{}, fmt.Errorf("catalog document key mapping %q does not select a document", sourcePath)
		}
	}

	supportFiles, err := captureSupportFiles(ctx, documents, byPath, captured, size, read, budget)
	if err != nil {
		return domain.CatalogCandidate{}, err
	}
	digest := catalogManifestDigest(documents, supportFiles, captured)
	revisionID := string(revisionKind) + "-sha256-" + digest
	if revisionKind == domain.CatalogRevisionGit {
		revisionID = "git-" + commit
	}
	candidate := domain.CatalogCandidate{
		ID: manifest.ID, Title: manifest.Title, Branding: manifest.Branding,
		DefaultDocumentKey: manifest.DefaultDocumentKey, ProfileID: manifest.ProfileID,
		Revision: domain.CatalogRevision{
			Kind: revisionKind, ID: revisionID, CommitSHA: commit, ManifestDigest: digest,
		},
		Documents: documents, SupportFiles: supportFiles,
	}
	if err := domain.ValidateCatalogCandidate(candidate); err != nil {
		return domain.CatalogCandidate{}, err
	}
	return candidate, nil
}

func validateCatalogManifest(manifest CatalogManifest) error {
	if err := domain.ValidateCatalogID(manifest.ID); err != nil {
		return err
	}
	if len(manifest.Includes) == 0 {
		return fmt.Errorf("catalog manifest requires include patterns")
	}
	for index, include := range manifest.Includes {
		if err := validateIncludePattern(include); err != nil {
			return fmt.Errorf("catalog include %d: %w", index, err)
		}
	}
	seen := make(map[string]struct{}, len(manifest.DocumentKeys))
	for index, mapping := range manifest.DocumentKeys {
		if err := validateSourcePath(fmt.Sprintf("catalog document key mapping %d path", index), mapping.SourcePath); err != nil {
			return err
		}
		if err := domain.ValidateCatalogDocumentKey(mapping.Key); err != nil {
			return err
		}
		if _, exists := seen[mapping.SourcePath]; exists {
			return fmt.Errorf("catalog document key mapping path %q is duplicated", mapping.SourcePath)
		}
		seen[mapping.SourcePath] = struct{}{}
	}
	return nil
}

func validateIncludePattern(pattern string) error {
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, `\`) || strings.ContainsRune(pattern, 0) || path.Clean(pattern) != pattern || pattern == "." || strings.HasPrefix(pattern, "../") {
		return fmt.Errorf("include pattern %q is not a clean relative slash path", pattern)
	}
	if _, err := path.Match(pattern, "probe"); err != nil {
		return fmt.Errorf("include pattern %q: %w", pattern, err)
	}
	return nil
}

func selectCatalogDocuments(patterns []string, inventory []catalogInventoryEntry) ([]catalogInventoryEntry, error) {
	matchedPattern := make([]bool, len(patterns))
	selected := make(map[string]catalogInventoryEntry)
	for _, entry := range inventory {
		for index, pattern := range patterns {
			matched, err := path.Match(pattern, entry.path)
			if err != nil {
				return nil, err
			}
			if matched {
				matchedPattern[index] = true
				selected[entry.path] = entry
			}
		}
	}
	for index, matched := range matchedPattern {
		if !matched {
			return nil, fmt.Errorf("catalog include %q matched no files", patterns[index])
		}
	}
	result := make([]catalogInventoryEntry, 0, len(selected))
	for _, entry := range selected {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result, nil
}

func validateCapturedFile(file capturedCatalogFile, expected catalogInventoryEntry) error {
	if file.path != expected.path || file.mode != expected.mode {
		return fmt.Errorf("captured file identity changed for %q", expected.path)
	}
	if len(file.data) == 0 {
		return fmt.Errorf("captured file %q is empty", expected.path)
	}
	if len(file.data) > maxCatalogSourceFileBytes {
		return fmt.Errorf("captured file %q exceeds %d bytes", expected.path, maxCatalogSourceFileBytes)
	}
	if int64(len(file.data)) != expected.size {
		return fmt.Errorf("captured file %q changed length", expected.path)
	}
	return nil
}

func captureSupportFiles(
	ctx context.Context,
	documents []domain.CatalogDocument,
	inventory map[string]catalogInventoryEntry,
	captured map[string]capturedCatalogFile,
	size catalogFileSizer,
	read catalogFileReader,
	budget *catalogSourceBudget,
) ([]domain.CatalogSupportFile, error) {
	queue := make([]string, 0, len(documents))
	for _, document := range documents {
		queue = append(queue, document.SourcePath)
	}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		owner := queue[0]
		queue = queue[1:]
		refs, err := catalogRelativeReferences(owner, captured[owner].data)
		if err != nil {
			return nil, fmt.Errorf("inspect references in %q: %w", owner, err)
		}
		for _, ref := range refs {
			if _, exists := captured[ref]; exists {
				continue
			}
			entry, exists := inventory[ref]
			if !exists {
				return nil, fmt.Errorf("captured reference %q from %q is undeclared or missing", ref, owner)
			}
			verifiedSize, err := size(ctx, entry)
			if err != nil {
				return nil, fmt.Errorf("size captured reference %q: %w", ref, err)
			}
			if err := budget.reserve(verifiedSize); err != nil {
				return nil, fmt.Errorf("admit captured reference %q: %w", ref, err)
			}
			entry.size = verifiedSize
			file, err := read(ctx, entry)
			if err != nil {
				return nil, fmt.Errorf("read captured reference %q: %w", ref, err)
			}
			if err := validateCapturedFile(file, entry); err != nil {
				return nil, err
			}
			captured[ref] = file
			queue = append(queue, ref)
		}
	}
	documentPaths := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		documentPaths[document.SourcePath] = struct{}{}
	}
	paths := make([]string, 0, len(captured))
	for sourcePath := range captured {
		if _, document := documentPaths[sourcePath]; !document {
			paths = append(paths, sourcePath)
		}
	}
	sort.Strings(paths)
	result := make([]domain.CatalogSupportFile, len(paths))
	for index, sourcePath := range paths {
		result[index] = domain.CatalogSupportFile{SourcePath: sourcePath, Bytes: captured[sourcePath].data}
	}
	return result, nil
}

func catalogRelativeReferences(owner string, data []byte) ([]string, error) {
	var value any
	switch strings.ToLower(path.Ext(owner)) {
	case ".json":
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &value); err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	var rawRefs []string
	collectCatalogReferences(value, &rawRefs)
	resolved := make(map[string]struct{}, len(rawRefs))
	for _, raw := range rawRefs {
		_, external, err := resolveCatalogReference(owner, raw)
		if err != nil {
			return nil, err
		}
		if external != "" {
			resolved[external] = struct{}{}
		}
	}
	result := make([]string, 0, len(resolved))
	for ref := range resolved {
		result = append(result, ref)
	}
	sort.Strings(result)
	return result, nil
}

func collectCatalogReferences(value any, refs *[]string) {
	switch value := value.(type) {
	case map[string]any:
		if raw, exists := value["$ref"]; exists {
			if ref, ok := raw.(string); ok {
				*refs = append(*refs, ref)
			}
		}
		for _, child := range value {
			collectCatalogReferences(child, refs)
		}
	case []any:
		for _, child := range value {
			collectCatalogReferences(child, refs)
		}
	}
}

func resolveCatalogReference(owner, raw string) (string, string, error) {
	if raw == "" {
		return raw, "", fmt.Errorf("empty OpenAPI reference")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, "", fmt.Errorf("invalid OpenAPI reference %q", raw)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" || parsed.RawQuery != "" || strings.Contains(raw, "%") || strings.Contains(raw, `\`) {
		return raw, "", fmt.Errorf("remote or encoded OpenAPI reference %q is forbidden", raw)
	}
	if parsed.Path == "" {
		return raw, "", nil
	}
	if strings.HasPrefix(parsed.Path, "/") {
		return raw, "", fmt.Errorf("absolute OpenAPI reference %q is forbidden", raw)
	}
	resolved := path.Clean(path.Join(path.Dir(owner), parsed.Path))
	if resolved == "." || strings.HasPrefix(resolved, "../") {
		return raw, "", fmt.Errorf("OpenAPI reference %q escapes the catalog root", raw)
	}
	return raw, resolved, nil
}

func catalogDocumentKey(sourcePath string) (string, error) {
	value := strings.TrimSuffix(sourcePath, path.Ext(sourcePath))
	var result strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(character)
			separator = false
			continue
		}
		if character == '/' || character == '-' || character == '_' || character == '.' || unicode.IsSpace(character) {
			separator = true
			continue
		}
		return "", fmt.Errorf("catalog document path %q cannot produce a canonical key", sourcePath)
	}
	key := strings.Trim(result.String(), "-")
	if err := domain.ValidateCatalogDocumentKey(key); err != nil {
		return "", fmt.Errorf("catalog document path %q: %w", sourcePath, err)
	}
	return key, nil
}

func catalogFormat(sourcePath string) domain.CatalogFormat {
	if strings.EqualFold(path.Ext(sourcePath), ".json") {
		return domain.CatalogFormatJSON
	}
	return domain.CatalogFormatYAML
}

func validateSourcePath(name, value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || strings.ContainsRune(value, 0) || value == "." || path.Clean(value) != value || strings.HasPrefix(value, "../") {
		return fmt.Errorf("%s %q is not a clean relative slash path", name, value)
	}
	return nil
}

func catalogManifestDigest(documents []domain.CatalogDocument, supports []domain.CatalogSupportFile, captured map[string]capturedCatalogFile) string {
	type record struct {
		role string
		key  string
		path string
		mode string
		data []byte
	}
	records := make([]record, 0, len(documents)+len(supports))
	for _, document := range documents {
		records = append(records, record{role: "document", key: document.Key, path: document.SourcePath, mode: captured[document.SourcePath].mode, data: document.Bytes})
	}
	for _, support := range supports {
		records = append(records, record{role: "support", path: support.SourcePath, mode: captured[support.SourcePath].mode, data: support.Bytes})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].path < records[j].path })
	hash := sha256.New()
	hash.Write([]byte("manja.renderer.source-manifest.v1"))
	for _, record := range records {
		writeManifestString(hash, record.role)
		writeManifestString(hash, record.key)
		writeManifestString(hash, record.path)
		writeManifestString(hash, record.mode)
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(record.data)))
		hash.Write(length[:])
		digest := sha256.Sum256(record.data)
		hash.Write(digest[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func writeManifestString(writer interface{ Write([]byte) (int, error) }, value string) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write([]byte(value))
}

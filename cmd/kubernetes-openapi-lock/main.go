package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- Git object identity is defined as SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/araihu/manja/domain"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

const (
	catalogSchemaVersion = 1
	receiptSchemaVersion = 1
	keyAlgorithm         = "kubernetes-openapi-v3-key-v1"
	generatedBegin       = "  # BEGIN generated Kubernetes OpenAPI v3"
	generatedEnd         = "  # END generated Kubernetes OpenAPI v3"
)

type inventoryEntry struct {
	Name       string
	Type       string
	Size       int64
	GitBlobSHA string
}

type sourceCatalog struct {
	SchemaVersion int              `json:"schemaVersion"`
	KeyAlgorithm  string           `json:"keyAlgorithm"`
	CommitSHA     string           `json:"commitSha"`
	Documents     []sourceDocument `json:"documents"`
	License       sourceArtifact   `json:"license"`
}

type sourceDocument struct {
	Key          string `json:"key"`
	UpstreamPath string `json:"upstreamPath"`
	Size         int64  `json:"size"`
	GitBlobSHA   string `json:"gitBlobSha"`
}

type sourceArtifact struct {
	UpstreamPath string `json:"upstreamPath"`
	Size         int64  `json:"size"`
	GitBlobSHA   string `json:"gitBlobSha"`
}

func (artifact sourceArtifact) inventory() inventoryEntry {
	return inventoryEntry{
		Name:       filepath.Base(artifact.UpstreamPath),
		Type:       "file",
		Size:       artifact.Size,
		GitBlobSHA: artifact.GitBlobSHA,
	}
}

type receipt struct {
	SchemaVersion       int               `json:"schemaVersion"`
	CommitSHA           string            `json:"commitSha"`
	SourceCatalogSHA256 string            `json:"sourceCatalogSha256"`
	Documents           []documentReceipt `json:"documents"`
	License             artifactReceipt   `json:"license"`
	DocumentCount       int               `json:"documentCount"`
	TotalBytes          int64             `json:"totalBytes"`
	OperationCount      int               `json:"operationCount"`
	DocumentSchemaCount int               `json:"documentSchemaCount"`
	UniqueSchemaCount   int               `json:"uniqueSchemaCount"`
}

type documentReceipt struct {
	Key            string `json:"key"`
	UpstreamPath   string `json:"upstreamPath"`
	Size           int64  `json:"size"`
	GitBlobSHA     string `json:"gitBlobSha"`
	SHA256         string `json:"sha256"`
	OperationCount int    `json:"operationCount"`
	SchemaCount    int    `json:"schemaCount"`
}

type artifactReceipt struct {
	UpstreamPath string `json:"upstreamPath"`
	Size         int64  `json:"size"`
	GitBlobSHA   string `json:"gitBlobSha"`
	SHA256       string `json:"sha256"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}

	var err error
	switch args[0] {
	case "catalog":
		err = runCatalog(args[1:], stdout, stderr)
	case "receipt":
		err = runReceipt(args[1:], stdout, stderr)
	case "allowlist":
		err = runAllowlist(args[1:], stdout, stderr)
	default:
		writeUsage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func writeUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: kubernetes-openapi-lock <catalog|allowlist|receipt> [flags]")
}

func runCatalog(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("catalog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	commit := flags.String("commit", "", "full upstream commit SHA")
	inventoryPath := flags.String("inventory", "", "tab-separated upstream inventory")
	licenseSize := flags.Int64("license-size", 0, "upstream LICENSE byte size")
	licenseBlob := flags.String("license-git-blob", "", "upstream LICENSE Git blob SHA")
	catalogPath := flags.String("catalog", "", "catalog-source.json output")
	manifestPath := flags.String("muamba", "", "Muamba manifest to update")
	allowlistPath := flags.String("allowlist", "", "default allowlist path to bootstrap when absent")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *commit == "" || *inventoryPath == "" || *catalogPath == "" || *manifestPath == "" || *allowlistPath == "" {
		return errors.New("catalog requires -commit, -inventory, -license-size, -license-git-blob, -catalog, -muamba, and -allowlist")
	}

	inventoryFile, err := os.Open(*inventoryPath)
	if err != nil {
		return fmt.Errorf("open inventory: %w", err)
	}
	entries, parseErr := parseInventory(inventoryFile)
	closeErr := inventoryFile.Close()
	if parseErr != nil {
		return parseErr
	}
	if closeErr != nil {
		return fmt.Errorf("close inventory: %w", closeErr)
	}
	catalog, err := buildSourceCatalog(*commit, entries, inventoryEntry{
		Name: "LICENSE", Type: "file", Size: *licenseSize, GitBlobSHA: *licenseBlob,
	})
	if err != nil {
		return err
	}
	catalogBytes, err := encodeSourceCatalog(catalog)
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fmt.Errorf("read Muamba manifest: %w", err)
	}
	manifestBytes, err = replaceGeneratedMuambaResource(manifestBytes, catalog)
	if err != nil {
		return err
	}
	for _, output := range []struct {
		path string
		data []byte
	}{
		{path: *catalogPath, data: catalogBytes},
		{path: *manifestPath, data: manifestBytes},
	} {
		if err := writeFile(output.path, output.data); err != nil {
			return err
		}
	}
	if _, err := os.Stat(*allowlistPath); errors.Is(err, os.ErrNotExist) {
		allowlistBytes := []byte("{\n  \"schemaVersion\": 1,\n  \"diagnostics\": []\n}\n")
		if err := writeFile(*allowlistPath, allowlistBytes); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("stat default allowlist: %w", err)
	}
	fmt.Fprintf(stdout, "catalog: %d documents at %s\n", len(catalog.Documents), catalog.CommitSHA)
	return nil
}

func runReceipt(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("receipt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", "", "catalog-source.json input")
	specRoot := flags.String("spec-root", "", "locked specification directory")
	licensePath := flags.String("license", "", "locked LICENSE path")
	outPath := flags.String("out", "", "receipt output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *catalogPath == "" || *specRoot == "" || *licensePath == "" || *outPath == "" {
		return errors.New("receipt requires -catalog, -spec-root, -license, and -out")
	}
	catalog, _, err := loadSourceCatalog(*catalogPath)
	if err != nil {
		return err
	}
	value, err := buildReceipt(catalog, *specRoot, *licensePath)
	if err != nil {
		return err
	}
	data, err := encodeReceipt(value)
	if err != nil {
		return err
	}
	if err := writeFile(*outPath, data); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "receipt: %d documents, %d operations, %d schemas\n", value.DocumentCount, value.OperationCount, value.DocumentSchemaCount)
	return nil
}

func runAllowlist(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("allowlist", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", "", "catalog-source.json input")
	specRoot := flags.String("spec-root", "", "locked specification directory")
	outPath := flags.String("out", "", "default allowlist output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *catalogPath == "" || *specRoot == "" || *outPath == "" {
		return errors.New("allowlist requires -catalog, -spec-root, and -out")
	}
	catalog, _, err := loadSourceCatalog(*catalogPath)
	if err != nil {
		return err
	}
	documents := make([]domain.CatalogDocument, len(catalog.Documents))
	for index, document := range catalog.Documents {
		data, err := os.ReadFile(filepath.Join(*specRoot, filepath.Base(document.UpstreamPath)))
		if err != nil {
			return fmt.Errorf("read %s: %w", document.UpstreamPath, err)
		}
		if int64(len(data)) != document.Size {
			return fmt.Errorf("%s size = %d, want %d", document.UpstreamPath, len(data), document.Size)
		}
		if err := verifyGitBlob(document.GitBlobSHA, data); err != nil {
			return fmt.Errorf("%s: %w", document.UpstreamPath, err)
		}
		documents[index] = domain.CatalogDocument{
			Key: document.Key, SourcePath: "specs/" + filepath.Base(document.UpstreamPath),
			Format: domain.CatalogFormatJSON, Bytes: data,
		}
	}
	data, err := openapiadapter.BuildKubernetesDefaultAllowlist(context.Background(), documents)
	if err != nil {
		return err
	}
	if err := writeFile(*outPath, data); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "allowlist: %d documents\n", len(documents))
	return nil
}

func parseInventory(reader io.Reader) ([]inventoryEntry, error) {
	scanner := bufio.NewScanner(reader)
	var entries []inventoryEntry
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			return nil, fmt.Errorf("inventory line %d: want four tab-separated columns", lineNumber)
		}
		size, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || size <= 0 {
			return nil, fmt.Errorf("inventory line %d: invalid positive size", lineNumber)
		}
		entry := inventoryEntry{Name: parts[0], Type: parts[1], Size: size, GitBlobSHA: parts[3]}
		if entry.Type != "file" {
			return nil, fmt.Errorf("inventory line %d: non-file entry", lineNumber)
		}
		if entry.Name == "" || filepath.Base(entry.Name) != entry.Name || strings.Contains(entry.Name, "\\") {
			return nil, fmt.Errorf("inventory line %d: invalid filename", lineNumber)
		}
		if !validHexDigest(entry.GitBlobSHA, 40) {
			return nil, fmt.Errorf("inventory line %d: invalid Git blob SHA", lineNumber)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read inventory: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("inventory is empty")
	}
	return entries, nil
}

func documentKey(filename string) (string, error) {
	if !strings.HasSuffix(filename, "_openapi.json") || filepath.Base(filename) != filename {
		return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
	}
	stem := strings.TrimSuffix(filename, "_openapi.json")
	parts := strings.Split(stem, "__")
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
		}

	}
	var key string
	switch parts[0] {
	case ".well-known":
		if len(parts) != 2 || parts[1] != "openid-configuration" {
			return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
		}
		key = "well-known-openid-configuration"
	case "api":
		switch len(parts) {
		case 1:
			key = "core-discovery"
		case 2:
			key = "core-" + parts[1]
		default:
			return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
		}
	case "apis":
		switch len(parts) {
		case 1:
			key = "apis-discovery"
		case 2:
			key = normalizeGroup(parts[1]) + "-discovery"
		case 3:
			key = normalizeGroup(parts[1]) + "-" + parts[2]
		default:
			return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
		}
	case "logs", "version":
		if len(parts) != 1 {
			return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
		}
		key = parts[0]
	case "openid":
		if len(parts) != 3 || parts[1] != "v1" || parts[2] != "jwks" {
			return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
		}
		key = "openid-v1-jwks"
	default:
		return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
	}
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("unsupported Kubernetes OpenAPI filename %q", filename)
	}
	return key, nil
}

func normalizeGroup(group string) string {
	group = strings.TrimSuffix(group, ".k8s.io")
	return strings.ReplaceAll(group, ".", "-")
}

func responseSizeCeiling(size int64) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("size must be positive: %d", size)
	}
	for _, limit := range []struct {
		bytes int64
		name  string
	}{
		{4 << 10, "4KiB"},
		{16 << 10, "16KiB"},
		{64 << 10, "64KiB"},
		{256 << 10, "256KiB"},
		{1 << 20, "1MiB"},
		{4 << 20, "4MiB"},
	} {
		if size <= limit.bytes {
			return limit.name, nil
		}
	}
	return "", fmt.Errorf("size %d exceeds 4MiB", size)
}

func buildSourceCatalog(commit string, entries []inventoryEntry, license inventoryEntry) (sourceCatalog, error) {
	if !validHexDigest(commit, 40) {
		return sourceCatalog{}, errors.New("Kubernetes commit must be a full lowercase 40-hex SHA")
	}
	if len(entries) != 65 {
		return sourceCatalog{}, fmt.Errorf("Kubernetes inventory has %d documents, want 65", len(entries))
	}
	if license.Name != "LICENSE" || license.Type != "file" || license.Size <= 0 || !validHexDigest(license.GitBlobSHA, 40) {
		return sourceCatalog{}, errors.New("invalid Kubernetes LICENSE inventory")
	}

	documents := make([]sourceDocument, 0, len(entries))
	names := make(map[string]struct{}, len(entries))
	keys := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Type != "file" || entry.Size <= 0 || !validHexDigest(entry.GitBlobSHA, 40) {
			return sourceCatalog{}, fmt.Errorf("invalid inventory entry %q", entry.Name)
		}
		if _, duplicate := names[entry.Name]; duplicate {
			return sourceCatalog{}, fmt.Errorf("duplicate inventory filename %q", entry.Name)
		}
		names[entry.Name] = struct{}{}
		key, err := documentKey(entry.Name)
		if err != nil {
			return sourceCatalog{}, err
		}
		if _, duplicate := keys[key]; duplicate {
			return sourceCatalog{}, fmt.Errorf("duplicate document key %q", key)
		}
		keys[key] = struct{}{}
		documents = append(documents, sourceDocument{
			Key:          key,
			UpstreamPath: "api/openapi-spec/v3/" + entry.Name,
			Size:         entry.Size,
			GitBlobSHA:   entry.GitBlobSHA,
		})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].UpstreamPath < documents[j].UpstreamPath })
	return sourceCatalog{
		SchemaVersion: catalogSchemaVersion,
		KeyAlgorithm:  keyAlgorithm,
		CommitSHA:     commit,
		Documents:     documents,
		License: sourceArtifact{
			UpstreamPath: "LICENSE",
			Size:         license.Size,
			GitBlobSHA:   license.GitBlobSHA,
		},
	}, nil
}

func encodeSourceCatalog(catalog sourceCatalog) ([]byte, error) {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode source catalog: %w", err)
	}
	return append(data, '\n'), nil
}

func loadSourceCatalog(path string) (sourceCatalog, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sourceCatalog{}, nil, fmt.Errorf("read source catalog: %w", err)
	}
	var catalog sourceCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return sourceCatalog{}, nil, fmt.Errorf("decode source catalog: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return sourceCatalog{}, nil, err
	}
	validated, err := catalogInventory(catalog)
	if err != nil {
		return sourceCatalog{}, nil, err
	}
	if validated.SchemaVersion != catalog.SchemaVersion || validated.KeyAlgorithm != catalog.KeyAlgorithm {
		return sourceCatalog{}, nil, errors.New("source catalog format identity mismatch")
	}
	canonical, err := encodeSourceCatalog(catalog)
	if err != nil {
		return sourceCatalog{}, nil, err
	}
	if !bytes.Equal(canonical, data) {
		return sourceCatalog{}, nil, errors.New("source catalog is not canonical")
	}
	return catalog, data, nil
}

func catalogInventory(catalog sourceCatalog) (sourceCatalog, error) {
	entries := make([]inventoryEntry, len(catalog.Documents))
	for index, document := range catalog.Documents {
		entries[index] = inventoryEntry{
			Name: filepath.Base(document.UpstreamPath), Type: "file", Size: document.Size, GitBlobSHA: document.GitBlobSHA,
		}
	}
	rebuilt, err := buildSourceCatalog(catalog.CommitSHA, entries, catalog.License.inventory())
	if err != nil {
		return sourceCatalog{}, err
	}
	if rebuilt.SchemaVersion != catalog.SchemaVersion || rebuilt.KeyAlgorithm != catalog.KeyAlgorithm {
		return sourceCatalog{}, errors.New("source catalog format identity mismatch")
	}
	for index := range rebuilt.Documents {
		if rebuilt.Documents[index] != catalog.Documents[index] {
			return sourceCatalog{}, fmt.Errorf("source catalog document %d is not canonical", index)
		}
	}
	if rebuilt.License != catalog.License {
		return sourceCatalog{}, errors.New("source catalog license is not canonical")
	}
	return rebuilt, nil
}

func replaceGeneratedMuambaResource(manifest []byte, catalog sourceCatalog) ([]byte, error) {
	block, err := encodeMuambaResource(catalog)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(manifest), "\n")
	begin := strings.Index(text, generatedBegin)
	end := strings.Index(text, generatedEnd)
	switch {
	case begin == -1 && end == -1:
		return []byte(text + "\n" + block), nil
	case begin == -1 || end == -1 || end < begin:
		return nil, errors.New("invalid generated Kubernetes Muamba markers")
	default:
		matches, err := muambaResourceMatches(manifest, catalog)
		if err != nil {
			return nil, err
		}
		if matches {
			return append([]byte(nil), manifest...), nil
		}
		end += len(generatedEnd)
		return []byte(strings.TrimRight(text[:begin], "\n") + "\n" + block + strings.TrimLeft(text[end:], "\n")), nil
	}
}

type muambaManifest struct {
	Resources map[string]muambaResource `yaml:"resources"`
}

type muambaResource struct {
	Version   string                    `yaml:"version"`
	Downloads map[string]muambaDownload `yaml:"downloads"`
}

type muambaDownload struct {
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	MaxSize string `yaml:"max_size"`
}

func muambaResourceMatches(manifest []byte, catalog sourceCatalog) (bool, error) {
	var parsed muambaManifest
	if err := yaml.Unmarshal(manifest, &parsed); err != nil {
		return false, fmt.Errorf("decode Muamba manifest: %w", err)
	}
	resource, exists := parsed.Resources["kubernetes-openapi-v3"]
	if !exists || resource.Version != catalog.CommitSHA || len(resource.Downloads) != len(catalog.Documents)+1 {
		return false, nil
	}
	for _, document := range catalog.Documents {
		download, exists := resource.Downloads[document.Key]
		if !exists {
			return false, nil
		}
		maxSize, err := responseSizeCeiling(document.Size)
		if err != nil {
			return false, err
		}
		if download.URL != "https://raw.githubusercontent.com/kubernetes/kubernetes/"+catalog.CommitSHA+"/"+document.UpstreamPath ||
			download.Path != "internal/renderer/testdata/kubernetes/specs/"+filepath.Base(document.UpstreamPath) ||
			download.MaxSize != maxSize {
			return false, nil
		}
	}
	license, exists := resource.Downloads["license"]
	if !exists {
		return false, nil
	}
	maxSize, err := responseSizeCeiling(catalog.License.Size)
	if err != nil {
		return false, err
	}
	return license.URL == "https://raw.githubusercontent.com/kubernetes/kubernetes/"+catalog.CommitSHA+"/LICENSE" &&
		license.Path == "internal/renderer/testdata/kubernetes/LICENSE" &&
		license.MaxSize == maxSize, nil
}

func encodeMuambaResource(catalog sourceCatalog) (string, error) {
	var builder strings.Builder
	builder.WriteString(generatedBegin + "\n")
	builder.WriteString("  kubernetes-openapi-v3:\n")
	fmt.Fprintf(&builder, "    version: %q\n", catalog.CommitSHA)
	builder.WriteString("    downloads:\n")
	for _, document := range catalog.Documents {
		maxSize, err := responseSizeCeiling(document.Size)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, "      %s:\n", document.Key)
		fmt.Fprintf(&builder, "        url: https://raw.githubusercontent.com/kubernetes/kubernetes/%s/%s\n", catalog.CommitSHA, document.UpstreamPath)
		fmt.Fprintf(&builder, "        path: internal/renderer/testdata/kubernetes/specs/%s\n", filepath.Base(document.UpstreamPath))
		fmt.Fprintf(&builder, "        max_size: %s\n", maxSize)
	}
	maxSize, err := responseSizeCeiling(catalog.License.Size)
	if err != nil {
		return "", err
	}
	builder.WriteString("      license:\n")
	fmt.Fprintf(&builder, "        url: https://raw.githubusercontent.com/kubernetes/kubernetes/%s/LICENSE\n", catalog.CommitSHA)
	builder.WriteString("        path: internal/renderer/testdata/kubernetes/LICENSE\n")
	fmt.Fprintf(&builder, "        max_size: %s\n", maxSize)
	builder.WriteString(generatedEnd + "\n")
	return builder.String(), nil
}

func buildReceipt(catalog sourceCatalog, specRoot, licensePath string) (receipt, error) {
	canonicalCatalog, err := encodeSourceCatalog(catalog)
	if err != nil {
		return receipt{}, err
	}
	value := receipt{
		SchemaVersion:       receiptSchemaVersion,
		CommitSHA:           catalog.CommitSHA,
		SourceCatalogSHA256: sha256Hex(canonicalCatalog),
		Documents:           make([]documentReceipt, 0, len(catalog.Documents)),
		DocumentCount:       len(catalog.Documents),
	}
	uniqueSchemas := make(map[string]struct{})
	for _, document := range catalog.Documents {
		path := filepath.Join(specRoot, filepath.Base(document.UpstreamPath))
		data, err := os.ReadFile(path)
		if err != nil {
			return receipt{}, fmt.Errorf("read %s: %w", document.UpstreamPath, err)
		}
		if int64(len(data)) != document.Size {
			return receipt{}, fmt.Errorf("%s size = %d, want %d", document.UpstreamPath, len(data), document.Size)
		}
		if err := verifyGitBlob(document.GitBlobSHA, data); err != nil {
			return receipt{}, fmt.Errorf("%s: %w", document.UpstreamPath, err)
		}
		operationCount, schemaNames, err := countOpenAPI(data)
		if err != nil {
			return receipt{}, fmt.Errorf("%s: %w", document.UpstreamPath, err)
		}
		for _, name := range schemaNames {
			uniqueSchemas[name] = struct{}{}
		}
		value.Documents = append(value.Documents, documentReceipt{
			Key: document.Key, UpstreamPath: document.UpstreamPath, Size: document.Size,
			GitBlobSHA: document.GitBlobSHA, SHA256: sha256Hex(data),
			OperationCount: operationCount, SchemaCount: len(schemaNames),
		})
		value.TotalBytes += document.Size
		value.OperationCount += operationCount
		value.DocumentSchemaCount += len(schemaNames)
	}
	license, err := os.ReadFile(licensePath)
	if err != nil {
		return receipt{}, fmt.Errorf("read LICENSE: %w", err)
	}
	if int64(len(license)) != catalog.License.Size {
		return receipt{}, fmt.Errorf("LICENSE size = %d, want %d", len(license), catalog.License.Size)
	}
	if err := verifyGitBlob(catalog.License.GitBlobSHA, license); err != nil {
		return receipt{}, fmt.Errorf("LICENSE: %w", err)
	}
	value.License = artifactReceipt{
		UpstreamPath: catalog.License.UpstreamPath,
		Size:         catalog.License.Size, GitBlobSHA: catalog.License.GitBlobSHA, SHA256: sha256Hex(license),
	}
	value.UniqueSchemaCount = len(uniqueSchemas)
	return value, nil
}

func countOpenAPI(data []byte) (int, []string, error) {
	var root map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&root); err != nil {
		return 0, nil, fmt.Errorf("decode OpenAPI: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return 0, nil, err
	}
	var paths map[string]map[string]json.RawMessage
	if raw, ok := root["paths"]; ok {
		if err := json.Unmarshal(raw, &paths); err != nil {
			return 0, nil, fmt.Errorf("decode paths: %w", err)
		}
	}
	methods := map[string]struct{}{
		"get": {}, "put": {}, "post": {}, "delete": {}, "options": {}, "head": {}, "patch": {}, "trace": {},
	}
	operationCount := 0
	for _, item := range paths {
		for method := range item {
			if _, isOperation := methods[strings.ToLower(method)]; isOperation {
				operationCount++
			}
		}
	}

	var components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	}
	if raw, ok := root["components"]; ok {
		if err := json.Unmarshal(raw, &components); err != nil {
			return 0, nil, fmt.Errorf("decode components: %w", err)
		}
	}
	schemaNames := make([]string, 0, len(components.Schemas))
	for name := range components.Schemas {
		schemaNames = append(schemaNames, name)
	}
	sort.Strings(schemaNames)
	return operationCount, schemaNames, nil
}

func encodeReceipt(value receipt) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode receipt: %w", err)
	}
	return append(data, '\n'), nil
}

func verifyGitBlob(expected string, data []byte) error {
	if !validHexDigest(expected, 40) {
		return errors.New("invalid expected Git blob SHA")
	}
	hash := sha1.New() // #nosec G401 -- Git object identity is defined as SHA-1.
	fmt.Fprintf(hash, "blob %d%c", len(data), byte(0))
	_, _ = hash.Write(data)
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("Git blob mismatch: expected %s, actual %s", expected, actual)
	}
	return nil
}

func validHexDigest(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

package distribution

// This file contains the mechanical packaging boundary. It deliberately
// accepts only bytes that a caller supplies and can inspect. It does not infer
// first-party ownership, write repository legal files, or turn a blocked
// authority receipt into an Apache claim.

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var epoch = time.Unix(0, 0).UTC()

// Inventory is the deterministic regular-file inventory of one complete root.
// Digest is a digest of the canonical inventory, not a claim about an archive
// or image manifest. Archive inspection uses the archive's raw-byte digest.
type Inventory struct {
	Files  []FileEvidence `json:"files"`
	Digest string         `json:"digest"`
}

// RootOptions controls complete-root inspection.
type RootOptions struct {
	// ExcludedPaths are slash-separated paths rejected recursively anywhere
	// below the root. A path is never silently omitted from an inventory.
	ExcludedPaths []string
}

// ArchiveOptions controls digest-bound, safe archive inspection.
type ArchiveOptions struct {
	ExcludedPaths []string
	// ExpectedDigest must be the independently supplied raw archive digest.
	// An archive without an expected digest is not authoritative.
	ExpectedDigest string
}

// InventoryError is a structured fail-closed root or archive inspection error.
type InventoryError struct {
	Code   string
	Path   string
	Detail string
}

func (e *InventoryError) Error() string {
	if e.Path == "" {
		return e.Code + ": " + e.Detail
	}
	return e.Code + " (" + e.Path + "): " + e.Detail
}

// InspectRoot recursively inventories a complete, caller-supplied artifact
// root. It rejects links and special files, follows no link, and never treats
// a selected child directory as an archive-wide inspection.
func InspectRoot(root string, options RootOptions) (Inventory, error) {
	if root == "" {
		return Inventory{}, &InventoryError{Code: "artifact.root.missing", Detail: "artifact root is required"}
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Inventory{}, &InventoryError{Code: "artifact.root.missing", Path: root, Detail: err.Error()}
		}
		return Inventory{}, &InventoryError{Code: "artifact.root.unreadable", Path: root, Detail: err.Error()}
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return Inventory{}, &InventoryError{Code: "artifact.root.link", Path: root, Detail: "artifact root must not be a symbolic link"}
	}
	if !rootInfo.IsDir() {
		return Inventory{}, &InventoryError{Code: "artifact.root.not_directory", Path: root, Detail: "artifact root must be a directory"}
	}

	excluded, err := normalizeExclusions(options.ExcludedPaths)
	if err != nil {
		return Inventory{}, err
	}
	files := make([]FileEvidence, 0)
	walkErr := filepath.WalkDir(root, func(pathValue string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return &InventoryError{Code: "artifact.root.walk_failed", Path: pathValue, Detail: walkErr.Error()}
		}
		if pathValue == root {
			return nil
		}
		relative, err := filepath.Rel(root, pathValue)
		if err != nil {
			return &InventoryError{Code: "artifact.path.relative_failed", Path: pathValue, Detail: err.Error()}
		}
		canonical, err := canonicalRelativePath(relative)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return &InventoryError{Code: "artifact.file.link", Path: canonical, Detail: "symbolic links are not accepted in an artifact root"}
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeType != 0 || !entry.Type().IsRegular() {
			return &InventoryError{Code: "artifact.file.type_invalid", Path: canonical, Detail: "artifact root contains a non-regular file"}
		}
		if excludedPath(canonical, excluded) {
			return &InventoryError{Code: "artifact.excluded_source", Path: canonical, Detail: "artifact root contains an excluded path"}
		}
		file, err := inventoryFile(pathValue, canonical)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if walkErr != nil {
		return Inventory{}, walkErr
	}
	if len(files) == 0 {
		return Inventory{}, &InventoryError{Code: "artifact.inventory.empty", Path: root, Detail: "artifact root contains no regular files"}
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	return Inventory{Files: files, Digest: digestInventory(files)}, nil
}

// InspectArchive verifies an independently expected archive digest, extracts
// every archive entry into a fresh private root, and recursively inventories
// that extracted root. It accepts regular files and directory entries only;
// links and special entries are rejected before any bytes can escape.
func InspectArchive(archivePath string, options ArchiveOptions) (Inventory, error) {
	if archivePath == "" {
		return Inventory{}, &InventoryError{Code: "artifact.archive.missing", Detail: "archive path is required"}
	}
	if !validDigest(options.ExpectedDigest) {
		return Inventory{}, &InventoryError{Code: "artifact.archive.digest_missing", Detail: "archive inspection requires a lowercase sha256 or sha384 expected digest"}
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return Inventory{}, &InventoryError{Code: "artifact.archive.unreadable", Path: archivePath, Detail: err.Error()}
	}
	actualDigest := digestForExpected(data, options.ExpectedDigest)
	if actualDigest != options.ExpectedDigest {
		return Inventory{}, &InventoryError{Code: "artifact.archive.digest_mismatch", Path: archivePath, Detail: fmt.Sprintf("expected %s, got %s", options.ExpectedDigest, actualDigest)}
	}
	excluded, err := normalizeExclusions(options.ExcludedPaths)
	if err != nil {
		return Inventory{}, err
	}
	freshRoot, err := os.MkdirTemp("", "manja-distribution-root-")
	if err != nil {
		return Inventory{}, &InventoryError{Code: "artifact.root.create_failed", Detail: err.Error()}
	}
	defer os.RemoveAll(freshRoot)

	reader := tar.NewReader(bytes.NewReader(data))
	entries := make(map[string]byte)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Inventory{}, &InventoryError{Code: "artifact.archive.parse_failed", Path: archivePath, Detail: err.Error()}
		}
		canonical, err := canonicalArchivePath(header.Name)
		if err != nil {
			return Inventory{}, err
		}
		if _, exists := entries[canonical]; exists {
			return Inventory{}, &InventoryError{Code: "artifact.archive.duplicate", Path: canonical, Detail: "archive entry is duplicated"}
		}
		entries[canonical] = header.Typeflag
		switch header.Typeflag {
		case tar.TypeDir:
			if canonical == "" {
				return Inventory{}, &InventoryError{Code: "artifact.archive.path_invalid", Detail: "archive root directory entry is not allowed"}
			}
			if err := os.MkdirAll(filepath.Join(freshRoot, filepath.FromSlash(canonical)), 0o755); err != nil {
				return Inventory{}, &InventoryError{Code: "artifact.archive.extract_failed", Path: canonical, Detail: err.Error()}
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return Inventory{}, &InventoryError{Code: "artifact.archive.size_invalid", Path: canonical, Detail: "archive regular-file size is negative"}
			}
			if excludedPath(canonical, excluded) {
				return Inventory{}, &InventoryError{Code: "artifact.excluded_source", Path: canonical, Detail: "archive contains an excluded path"}
			}
			outputPath := filepath.Join(freshRoot, filepath.FromSlash(canonical))
			if err := ensureWithinRoot(freshRoot, outputPath); err != nil {
				return Inventory{}, err
			}
			if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
				return Inventory{}, &InventoryError{Code: "artifact.archive.extract_failed", Path: canonical, Detail: err.Error()}
			}
			file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return Inventory{}, &InventoryError{Code: "artifact.archive.extract_failed", Path: canonical, Detail: err.Error()}
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return Inventory{}, &InventoryError{Code: "artifact.archive.extract_failed", Path: canonical, Detail: copyErr.Error()}
			}
			if closeErr != nil {
				return Inventory{}, &InventoryError{Code: "artifact.archive.extract_failed", Path: canonical, Detail: closeErr.Error()}
			}
			if err := os.Chmod(outputPath, os.FileMode(header.Mode)&os.ModePerm); err != nil {
				return Inventory{}, &InventoryError{Code: "artifact.archive.extract_failed", Path: canonical, Detail: err.Error()}
			}
		default:
			return Inventory{}, &InventoryError{Code: "artifact.archive.entry_invalid", Path: canonical, Detail: "archive links and special entries are not accepted"}
		}
	}
	// The root is created by this function and is therefore a fresh complete
	// extraction root. InspectRoot repeats all path, type, and read checks.
	return InspectRoot(freshRoot, RootOptions{ExcludedPaths: options.ExcludedPaths})
}

// CompareInventory returns deterministic drift findings between an expected
// receipt and the actual complete-root inventory.
func CompareInventory(expected, actual []FileEvidence) []Finding {
	left := make(map[string]FileEvidence, len(expected))
	right := make(map[string]FileEvidence, len(actual))
	findings := make([]Finding, 0)
	seenExpected := make(map[string]struct{}, len(expected))
	for _, file := range expected {
		if _, exists := left[file.Path]; exists {
			findings = append(findings, Finding{Code: "artifact.drift.duplicate", Subject: file.Path, Detail: "expected artifact receipt contains a duplicate path"})
		}
		if _, exists := seenExpected[file.Path]; exists {
			continue
		}
		seenExpected[file.Path] = struct{}{}
		left[file.Path] = file
	}
	seenActual := make(map[string]struct{}, len(actual))
	for _, file := range actual {
		if _, exists := seenActual[file.Path]; exists {
			findings = append(findings, Finding{Code: "artifact.drift.duplicate", Subject: file.Path, Detail: "actual artifact inventory contains a duplicate path"})
		}
		seenActual[file.Path] = struct{}{}
		right[file.Path] = file
	}
	for pathValue, file := range left {
		actualFile, exists := right[pathValue]
		if !exists {
			findings = append(findings, Finding{Code: "artifact.drift.missing", Subject: pathValue, Detail: "expected artifact file is absent from the complete root"})
			continue
		}
		if file.Type != actualFile.Type || file.Size != actualFile.Size || file.Mode != actualFile.Mode || file.Digest != actualFile.Digest {
			findings = append(findings, Finding{Code: "artifact.drift.changed", Subject: pathValue, Detail: "artifact file metadata or bytes differ from the expected receipt"})
		}
	}
	for pathValue := range right {
		if _, exists := left[pathValue]; !exists {
			findings = append(findings, Finding{Code: "artifact.drift.extra", Subject: pathValue, Detail: "complete root contains a file absent from the expected receipt"})
		}
	}
	sortFindings(findings)
	return findings
}

// ValidateArtifactRoot checks one evidence record against an actual complete
// root. It is independent of authority evaluation: mechanical drift is
// reported even while legal authority remains blocked.
func ValidateArtifactRoot(root string, artifact ArtifactEvidence, policy Policy) (Inventory, []Finding) {
	inventory, err := InspectRoot(root, RootOptions{ExcludedPaths: runtimeExclusions(artifact.Kind, policy)})
	if err != nil {
		finding := Finding{Code: "artifact.inventory.inspect_failed", Subject: artifact.Name, Detail: err.Error()}
		if inventoryError, ok := err.(*InventoryError); ok && inventoryError.Code != "" {
			finding.Code = inventoryError.Code
		}
		return Inventory{}, []Finding{finding}
	}
	findings := CompareInventory(artifact.Files, inventory.Files)
	if artifact.Digest != "" && artifact.Digest != inventory.Digest {
		findings = append(findings, Finding{Code: "artifact.root.digest_mismatch", Subject: artifact.Name, Detail: fmt.Sprintf("expected %s, got %s", artifact.Digest, inventory.Digest)})
	}
	sortFindings(findings)
	return inventory, findings
}

// SBOMComponent is the stable, license-bearing CycloneDX component subset
// emitted by GenerateSBOM. It contains no timestamps, UUIDs, or environment
// paths, so equal dependency evidence produces equal bytes.
type SBOMComponent struct {
	Type     string        `json:"type"`
	BomRef   string        `json:"bom-ref"`
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Scope    string        `json:"scope"`
	Purl     string        `json:"purl,omitempty"`
	Licenses []SBOMLicense `json:"licenses"`
	Hashes   []SBOMHash    `json:"hashes"`
}

type SBOMLicense struct {
	License SBOMLicenseID `json:"license"`
}

type SBOMLicenseID struct {
	ID string `json:"id"`
}

type SBOMHash struct {
	Algorithm string `json:"alg"`
	Content   string `json:"content"`
}

type sbomDocument struct {
	BomFormat   string `json:"bomFormat"`
	SpecVersion string `json:"specVersion"`
	Version     int    `json:"version"`
	Metadata    struct {
		Component struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Version string `json:"version,omitempty"`
		} `json:"component"`
	} `json:"metadata"`
	Components []SBOMComponent `json:"components"`
}

// GenerateSBOM emits deterministic CycloneDX JSON and its digest-bound
// evidence. Unknown or missing dependency licenses fail before any bytes are
// returned; generated output never supplies first-party authority.
func GenerateSBOM(name, version string, dependencies []DependencyEvidence) ([]byte, SBOMEvidence, error) {
	if strings.TrimSpace(name) == "" {
		return nil, SBOMEvidence{}, errors.New("SBOM name is required")
	}
	components := make([]SBOMComponent, 0, len(dependencies))
	for _, dependency := range dependencies {
		if isUnknownLicense(dependency.License) {
			return nil, SBOMEvidence{}, fmt.Errorf("dependency %q has unknown or missing license", dependency.Name)
		}
		if dependency.Scope != ScopeShipped {
			return nil, SBOMEvidence{}, fmt.Errorf("dependency %q is not in shipped scope", dependency.Name)
		}
		if dependency.Ecosystem == "" || dependency.Name == "" || dependency.Version == "" {
			return nil, SBOMEvidence{}, fmt.Errorf("dependency identity is incomplete for %q", dependency.Name)
		}
		if !validDigest(dependency.Digest) {
			return nil, SBOMEvidence{}, fmt.Errorf("dependency %q has invalid digest", dependency.Name)
		}
		purl := ""
		if dependency.Ecosystem == "go" {
			purl = "pkg:golang/" + dependency.Name + "@" + dependency.Version
		} else if dependency.Ecosystem == "npm" {
			purl = "pkg:npm/" + dependency.Name + "@" + dependency.Version
		}
		hashAlgorithm := "SHA-256"
		hashContent := strings.TrimPrefix(dependency.Digest, "sha256:")
		if strings.HasPrefix(dependency.Digest, "sha384:") {
			hashAlgorithm = "SHA-384"
			hashContent = strings.TrimPrefix(dependency.Digest, "sha384:")
		}
		components = append(components, SBOMComponent{
			Type:     "library",
			BomRef:   dependency.Ecosystem + ":" + dependency.Name + "@" + dependency.Version,
			Name:     dependency.Name,
			Version:  dependency.Version,
			Scope:    string(dependency.Scope),
			Purl:     purl,
			Licenses: []SBOMLicense{{License: SBOMLicenseID{ID: strings.TrimSpace(dependency.License)}}},
			Hashes:   []SBOMHash{{Algorithm: hashAlgorithm, Content: hashContent}},
		})
	}
	sort.Slice(components, func(left, right int) bool { return components[left].BomRef < components[right].BomRef })
	document := sbomDocument{BomFormat: "CycloneDX", SpecVersion: "1.5", Version: 1, Components: components}
	document.Metadata.Component.Type = "application"
	document.Metadata.Component.Name = name
	document.Metadata.Component.Version = version
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, SBOMEvidence{}, fmt.Errorf("marshal SBOM: %w", err)
	}
	encoded = append(encoded, '\n')
	return encoded, SBOMEvidence{Format: "CycloneDX-JSON", Digest: sha256Digest(encoded), Complete: true}, nil
}

// ArtifactRequest describes one actual root to package. RootDigest, when
// supplied, must equal the canonical complete-root digest before packaging.
// ExpectedDigest must be an independently supplied digest of the resulting
// deterministic tar bytes. The two checks prevent source-root and
// final-artifact substitution.
type ArtifactRequest struct {
	Name                     string             `json:"name"`
	Kind                     ArtifactKind       `json:"kind"`
	Source                   string             `json:"source"`
	Root                     string             `json:"root"`
	RootDigest               string             `json:"rootDigest,omitempty"`
	ExpectedDigest           string             `json:"expectedDigest,omitempty"`
	Dependencies             []string           `json:"dependencies"`
	Platforms                []PlatformEvidence `json:"platforms,omitempty"`
	PlatformCoverageComplete bool               `json:"platformCoverageComplete,omitempty"`
}

// PackageRequest contains only caller-supplied authority and artifact input.
// LegalRoot is read only and is used solely after both authority receipts
// pass. A blocked request never writes OutputDir.
type PackageRequest struct {
	Subject      SubjectEvidence
	Provenance   AuthorityEvidence
	RightsHolder AuthorityEvidence
	Legal        LegalEvidence
	LegalRoot    string
	Dependencies []DependencyEvidence
	Artifacts    []ArtifactRequest
	OutputDir    string
}

// PackagedArtifact records one deterministic output and its final inventory.
type PackagedArtifact struct {
	Name     string           `json:"name"`
	Kind     ArtifactKind     `json:"kind"`
	Path     string           `json:"path"`
	Digest   string           `json:"digest"`
	Evidence ArtifactEvidence `json:"evidence"`
}

// PackageResult contains mechanical evidence even when authority blocks
// output. Outputs stays empty unless the whole request passes.
type PackageResult struct {
	Evidence Evidence           `json:"evidence"`
	Result   Result             `json:"result"`
	Outputs  []PackagedArtifact `json:"outputs"`
}

// Pack inspects every requested root, generates deterministic SBOM bytes in
// memory, and only writes final tar artifacts when provenance, rights-holder,
// dependency, notice, and inventory gates all pass.
func Pack(request PackageRequest, policy Policy) (PackageResult, error) {
	result := PackageResult{Outputs: []PackagedArtifact{}}
	if len(request.Artifacts) == 0 {
		result.Result = Result{Status: StatusBlocked, Findings: []Finding{{Code: "artifact.missing", Detail: "no produced artifact has been requested"}}}
		return result, nil
	}

	evidence := Evidence{
		SchemaVersion: evidenceSchemaVersion,
		Subject:       request.Subject,
		Provenance:    request.Provenance,
		RightsHolder:  request.RightsHolder,
		Legal:         request.Legal,
		Dependencies:  append([]DependencyEvidence(nil), request.Dependencies...),
	}
	authorityPassed := request.Provenance.Status == StatusPass && request.RightsHolder.Status == StatusPass
	mechanicalFindings := make([]Finding, 0)
	mechanicalFindings = append(mechanicalFindings, validateAuthority("provenance", request.Provenance)...)
	mechanicalFindings = append(mechanicalFindings, validateAuthority("rights_holder", request.RightsHolder)...)
	artifactNames := make(map[string]struct{}, len(request.Artifacts))
	for _, artifactRequest := range request.Artifacts {
		if !validArtifactName(artifactRequest.Name) {
			mechanicalFindings = append(mechanicalFindings, Finding{Code: "artifact.name.invalid", Subject: artifactRequest.Name, Detail: "artifact name must be one safe path segment"})
		}
		if _, exists := artifactNames[artifactRequest.Name]; exists && artifactRequest.Name != "" {
			mechanicalFindings = append(mechanicalFindings, Finding{Code: "artifact.duplicate", Subject: artifactRequest.Name, Detail: "artifact name is duplicated"})
		}
		artifactNames[artifactRequest.Name] = struct{}{}
		artifact, findings := inspectRequestedArtifact(artifactRequest, policy, request.Dependencies, authorityPassed)
		mechanicalFindings = append(mechanicalFindings, findings...)
		evidence.Artifacts = append(evidence.Artifacts, artifact)
	}
	// Preflight validates the actual roots, dependencies, SBOM inputs, and
	// exclusions before staging. Notice placement is intentionally deferred:
	// the final inventory does not exist until legal files and the generated
	// SBOM have been copied into a private staging root.
	dependencyByName, dependencyFindings := validateDependencies(request.Dependencies)
	mechanicalFindings = append(mechanicalFindings, dependencyFindings...)
	for _, artifact := range evidence.Artifacts {
		mechanicalFindings = append(mechanicalFindings, validateArtifact(artifact, policy, dependencyByName, request.Legal, false, authorityPassed, false)...)
	}
	if authorityPassed {
		checkedKinds := make(map[ArtifactKind]struct{}, len(request.Artifacts))
		for _, artifactRequest := range request.Artifacts {
			if _, checked := checkedKinds[artifactRequest.Kind]; !checked {
				checkedKinds[artifactRequest.Kind] = struct{}{}
				mechanicalFindings = append(mechanicalFindings, validateLegal(request.Legal, requiredPaths(policy, artifactRequest.Kind))...)
			}
			mechanicalFindings = append(mechanicalFindings, validateLegalAgainstRoot(artifactRequest, request.LegalRoot, request.Legal, policy)...)
		}
	}
	mechanicalFindings = append(mechanicalFindings, validateShippedDependencies(request.Dependencies, evidence.Artifacts)...)
	sortFindings(mechanicalFindings)
	result.Evidence = evidence
	if len(mechanicalFindings) > 0 || request.Provenance.Status != StatusPass || request.RightsHolder.Status != StatusPass {
		mechanical := Evaluate(evidence, policy)
		mechanical.Findings = append(mechanical.Findings, mechanicalFindings...)
		mechanical.Findings = deduplicateFindings(mechanical.Findings)
		mechanical.Status = StatusBlocked
		result.Result = mechanical
		return result, nil
	}
	if request.OutputDir == "" {
		result.Result = Result{Status: StatusBlocked, Findings: []Finding{{Code: "artifact.output.missing", Detail: "package output directory is required after all gates pass"}}}
		return result, errors.New("package output directory is required after all gates pass")
	}
	stagingOutput, err := os.MkdirTemp("", "manja-distribution-output-")
	if err != nil {
		return result, fmt.Errorf("create private package output directory: %w", err)
	}
	defer os.RemoveAll(stagingOutput)
	stagedRequest := request
	stagedRequest.OutputDir = stagingOutput
	prePackagingEvidence := evidence
	prePackagingEvidence.Artifacts = append([]ArtifactEvidence(nil), evidence.Artifacts...)

	for index, artifactRequest := range request.Artifacts {
		artifact := evidence.Artifacts[index]
		output, finalEvidence, err := packageOne(artifactRequest, artifact, stagedRequest, policy)
		if err != nil {
			result.Evidence = prePackagingEvidence
			result.Result = Result{Status: StatusBlocked, Findings: []Finding{{Code: "artifact.package.failed", Subject: artifactRequest.Name, Detail: err.Error()}}}
			result.Outputs = nil
			return result, nil
		}
		result.Outputs = append(result.Outputs, output)
		evidence.Artifacts[index] = finalEvidence
	}
	result.Evidence = evidence
	result.Result = Evaluate(evidence, policy)
	if result.Result.Status != StatusPass {
		result.Outputs = nil
		return result, nil
	}
	result.Outputs, err = publishOutputs(request.OutputDir, result.Outputs)
	if err != nil {
		result.Outputs = nil
		return result, fmt.Errorf("publish package outputs: %w", err)
	}
	return result, nil
}

func publishOutputs(outputDir string, outputs []PackagedArtifact) ([]PackagedArtifact, error) {
	outputDirExisted := false
	if info, err := os.Lstat(outputDir); err == nil {
		outputDirExisted = true
		if !info.IsDir() {
			return nil, fmt.Errorf("package output path is not a directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect package output directory: %w", err)
	} else if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create package output directory: %w", err)
	}
	published := make([]string, 0, len(outputs))
	cleanup := func() {
		for _, pathValue := range published {
			_ = os.Remove(pathValue)
		}
		if outputDirExisted {
			return
		}
		entries, err := os.ReadDir(outputDir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(outputDir)
		}
	}
	for index := range outputs {
		archiveBytes, err := os.ReadFile(outputs[index].Path)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("read staged package %q: %w", outputs[index].Name, err)
		}
		outputPath := filepath.Join(outputDir, outputs[index].Name+".tar")
		file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("create package %q: %w", outputs[index].Name, err)
		}
		written, writeErr := file.Write(archiveBytes)
		closeErr := file.Close()
		if writeErr != nil {
			_ = os.Remove(outputPath)
			cleanup()
			return nil, fmt.Errorf("write package %q: %w", outputs[index].Name, writeErr)
		}
		if written != len(archiveBytes) {
			_ = os.Remove(outputPath)
			cleanup()
			return nil, fmt.Errorf("write package %q: %w", outputs[index].Name, io.ErrShortWrite)
		}
		if closeErr != nil {
			_ = os.Remove(outputPath)
			cleanup()
			return nil, fmt.Errorf("close package %q: %w", outputs[index].Name, closeErr)
		}
		outputs[index].Path = outputPath
		published = append(published, outputPath)
	}
	return outputs, nil
}

func inspectRequestedArtifact(request ArtifactRequest, policy Policy, dependencies []DependencyEvidence, generateMaterials bool) (ArtifactEvidence, []Finding) {
	artifact := ArtifactEvidence{
		Name:                     request.Name,
		Kind:                     request.Kind,
		Source:                   request.Source,
		Digest:                   request.RootDigest,
		Inspection:               InspectionEvidence{Complete: true, FreshRoot: false, DigestBound: request.RootDigest != ""},
		Platforms:                append([]PlatformEvidence(nil), request.Platforms...),
		PlatformCoverageComplete: request.PlatformCoverageComplete,
		Dependencies:             append([]string(nil), request.Dependencies...),
	}
	inventory, err := InspectRoot(request.Root, RootOptions{ExcludedPaths: runtimeExclusions(request.Kind, policy)})
	if err != nil {
		finding := Finding{Code: "artifact.inventory.inspect_failed", Subject: request.Name, Detail: err.Error()}
		if inventoryError, ok := err.(*InventoryError); ok && inventoryError.Code != "" {
			finding.Code = inventoryError.Code
		}
		return artifact, []Finding{finding}
	}
	artifact.Files = inventory.Files
	if artifact.Digest == "" {
		artifact.Digest = inventory.Digest
	}
	findings := make([]Finding, 0)
	if request.Kind == ArtifactOCI {
		findings = append(findings, Finding{Code: "artifact.oci.real_artifact_missing", Subject: request.Name, Detail: "OCI packaging requires digest-bound image bytes; a caller directory cannot be converted to an OCI artifact"})
	}
	if request.RootDigest != "" && request.RootDigest != inventory.Digest {
		findings = append(findings, Finding{Code: "artifact.root.digest_mismatch", Subject: request.Name, Detail: fmt.Sprintf("expected %s, got %s", request.RootDigest, inventory.Digest)})
	}
	if request.ExpectedDigest != "" && !validDigest(request.ExpectedDigest) {
		findings = append(findings, Finding{Code: "artifact.digest.invalid", Subject: request.Name, Detail: "expected artifact digest must be immutable"})
	}
	if request.ExpectedDigest == "" {
		findings = append(findings, Finding{Code: "artifact.archive.digest_missing", Subject: request.Name, Detail: "packaging PASS requires an independently supplied expected archive digest"})
	}
	if generateMaterials {
		sbomBytes, sbom, err := GenerateSBOM(request.Name, "", dependenciesForArtifact(request.Dependencies, dependencies))
		if err != nil {
			findings = append(findings, Finding{Code: "artifact.sbom.license_invalid", Subject: request.Name, Detail: err.Error()})
		} else {
			artifact.SBOM = sbom
			sbomRelativePath, pathErr := sbomPath(policy, request.Kind, request.Name)
			if pathErr != nil {
				findings = append(findings, Finding{Code: "artifact.sbom.path_unsafe", Subject: request.Name, Detail: pathErr.Error()})
			} else {
				artifact.SBOM.Source = sbomRelativePath
				findings = append(findings, validateExistingSBOM(request.Root, artifact.SBOM.Source, sbomBytes, request.Name)...)
			}
		}
	}
	return artifact, findings
}

func validArtifactName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

func validateExistingSBOM(root, relativePath string, expected []byte, artifactName string) []Finding {
	pathValue := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := os.Lstat(pathValue)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []Finding{{Code: "artifact.sbom.unreadable", Subject: artifactName, Detail: err.Error()}}
	}
	if info.Mode()&os.ModeType != 0 || !info.Mode().IsRegular() {
		return []Finding{{Code: "artifact.sbom.incomplete", Subject: artifactName, Detail: "existing SBOM is not a regular file"}}
	}
	actual, err := os.ReadFile(pathValue)
	if err != nil {
		return []Finding{{Code: "artifact.sbom.unreadable", Subject: artifactName, Detail: err.Error()}}
	}
	if !sbomHasCompleteShape(actual, expected) {
		return []Finding{{Code: "artifact.sbom.incomplete", Subject: artifactName, Detail: "existing SBOM does not cover the generated component set"}}
	}
	if !bytes.Equal(actual, expected) {
		return []Finding{{Code: "artifact.sbom.bytes_mismatch", Subject: artifactName, Detail: "existing SBOM bytes differ from deterministic generated bytes"}}
	}
	return nil
}

func sbomHasCompleteShape(actual, expected []byte) bool {
	var actualDocument, expectedDocument sbomDocument
	if err := json.Unmarshal(actual, &actualDocument); err != nil {
		return false
	}
	if err := json.Unmarshal(expected, &expectedDocument); err != nil {
		return false
	}
	if actualDocument.BomFormat != "CycloneDX" || actualDocument.SpecVersion == "" || actualDocument.Version <= 0 {
		return false
	}
	if actualDocument.Metadata.Component.Type == "" || actualDocument.Metadata.Component.Name == "" {
		return false
	}
	if len(actualDocument.Components) != len(expectedDocument.Components) {
		return false
	}
	for index, component := range actualDocument.Components {
		if component.Type == "" || component.BomRef == "" || component.Name == "" || component.Version == "" || component.Scope != string(ScopeShipped) || len(component.Licenses) == 0 || len(component.Hashes) == 0 {
			return false
		}
		if component.BomRef != expectedDocument.Components[index].BomRef {
			return false
		}
	}
	return true
}

func validateLegalAgainstRoot(request ArtifactRequest, legalRoot string, legal LegalEvidence, policy Policy) []Finding {
	var findings []Finding
	for _, requiredPath := range requiredPaths(policy, request.Kind) {
		file, exists := legalFileForPath(legal, requiredPath)
		if !exists || isZeroFile(file) {
			continue
		}
		candidate := filepath.Join(request.Root, filepath.FromSlash(requiredPath))
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			if legalRoot == "" {
				findings = append(findings, Finding{Code: "legal.file.missing", Subject: request.Name + ":" + requiredPath, Detail: "required legal file is absent from artifact root and no legal source root was supplied"})
				continue
			}
			candidate = filepath.Join(legalRoot, filepath.Base(filepath.FromSlash(file.Path)))
		}
		actual, err := regularFileEvidence(candidate, requiredPath)
		if err != nil {
			findings = append(findings, Finding{Code: "legal.file.unreadable", Subject: request.Name + ":" + requiredPath, Detail: err.Error()})
			continue
		}
		if actual.Size != file.Size || actual.Digest != file.Digest {
			findings = append(findings, Finding{Code: "legal.file.digest_mismatch", Subject: request.Name + ":" + requiredPath, Detail: fmt.Sprintf("expected %d/%s, got %d/%s", file.Size, file.Digest, actual.Size, actual.Digest)})
		}
	}
	return findings
}

func packageOne(request ArtifactRequest, artifact ArtifactEvidence, packageRequest PackageRequest, policy Policy) (PackagedArtifact, ArtifactEvidence, error) {
	staging, err := os.MkdirTemp("", "manja-distribution-stage-")
	if err != nil {
		return PackagedArtifact{}, artifact, err
	}
	defer os.RemoveAll(staging)
	if err := copyRoot(request.Root, staging); err != nil {
		return PackagedArtifact{}, artifact, err
	}
	if err := installLegalFiles(staging, request.Kind, packageRequest.LegalRoot, packageRequest.Legal, policy); err != nil {
		return PackagedArtifact{}, artifact, err
	}
	sbomBytes, sbom, err := GenerateSBOM(request.Name, "", dependenciesForArtifact(request.Dependencies, packageRequest.Dependencies))
	if err != nil {
		return PackagedArtifact{}, artifact, err
	}
	sbomRelativePath, err := sbomPath(policy, request.Kind, request.Name)
	if err != nil {
		return PackagedArtifact{}, artifact, fmt.Errorf("invalid SBOM path: %w", err)
	}
	sbomFilePath := filepath.Join(staging, filepath.FromSlash(sbomRelativePath))
	if err := os.MkdirAll(filepath.Dir(sbomFilePath), 0o755); err != nil {
		return PackagedArtifact{}, artifact, err
	}
	if err := os.WriteFile(sbomFilePath, sbomBytes, 0o644); err != nil {
		return PackagedArtifact{}, artifact, err
	}
	finalInventory, err := InspectRoot(staging, RootOptions{ExcludedPaths: runtimeExclusions(request.Kind, policy)})
	if err != nil {
		return PackagedArtifact{}, artifact, err
	}
	artifact.Files = finalInventory.Files
	artifact.SBOM = sbom
	artifact.SBOM.Source = sbomRelativePath
	artifact.Digest = ""
	archiveBytes, err := deterministicTar(staging)
	if err != nil {
		return PackagedArtifact{}, artifact, err
	}
	archiveDigest := sha256Digest(archiveBytes)
	if request.ExpectedDigest != "" && request.ExpectedDigest != archiveDigest {
		return PackagedArtifact{}, artifact, fmt.Errorf("expected %s, got %s", request.ExpectedDigest, archiveDigest)
	}
	outputPath := filepath.Join(packageRequest.OutputDir, request.Name+".tar")
	if err := os.WriteFile(outputPath, archiveBytes, 0o644); err != nil {
		return PackagedArtifact{}, artifact, err
	}
	archiveInventory, err := InspectArchive(outputPath, ArchiveOptions{ExpectedDigest: archiveDigest, ExcludedPaths: runtimeExclusions(request.Kind, policy)})
	if err != nil {
		return PackagedArtifact{}, artifact, err
	}
	artifact.Digest = archiveDigest
	artifact.Files = archiveInventory.Files
	artifact.Inspection = InspectionEvidence{Complete: true, FreshRoot: true, DigestBound: true}
	return PackagedArtifact{Name: request.Name, Kind: request.Kind, Path: outputPath, Digest: archiveDigest, Evidence: artifact}, artifact, nil
}

func installLegalFiles(staging string, kind ArtifactKind, legalRoot string, legal LegalEvidence, policy Policy) error {
	for _, requiredPath := range requiredPaths(policy, kind) {
		file, exists := legalFileForPath(legal, requiredPath)
		if !exists || isZeroFile(file) {
			return fmt.Errorf("required legal file evidence %q is missing", requiredPath)
		}
		destination := filepath.Join(staging, filepath.FromSlash(requiredPath))
		if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
			if legalRoot == "" {
				return fmt.Errorf("required legal file %q is absent and no legal source root was supplied", requiredPath)
			}
			source := filepath.Join(legalRoot, filepath.Base(filepath.FromSlash(file.Path)))
			info, err := os.Lstat(source)
			if err != nil {
				return fmt.Errorf("read legal file %q: %w", requiredPath, err)
			}
			if info.Mode()&os.ModeType != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("legal file %q must be a regular file", file.Path)
			}
			data, err := os.ReadFile(source)
			if err != nil {
				return fmt.Errorf("read legal file %q: %w", requiredPath, err)
			}
			if int64(len(data)) != file.Size || digestForExpected(data, file.Digest) != file.Digest {
				return fmt.Errorf("legal file %q differs from supplied evidence", file.Path)
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(destination, data, 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		actual, err := regularFileEvidence(destination, requiredPath)
		if err != nil {
			return err
		}
		if actual.Size != file.Size || actual.Digest != file.Digest {
			return fmt.Errorf("legal file %q differs from supplied evidence", requiredPath)
		}
	}
	return nil
}

func copyRoot(source, destination string) error {
	_, err := InspectRoot(source, RootOptions{})
	if err != nil {
		return err
	}
	return filepath.WalkDir(source, func(pathValue string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if pathValue == source {
			return nil
		}
		relative, err := filepath.Rel(source, pathValue)
		if err != nil {
			return err
		}
		canonical, err := canonicalRelativePath(relative)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(canonical))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeType != 0 || !entry.Type().IsRegular() {
			return &InventoryError{Code: "artifact.file.type_invalid", Path: canonical, Detail: "only regular files can be copied"}
		}
		data, err := os.ReadFile(pathValue)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(target, info.Mode().Perm())
	})
}

func deterministicTar(root string) ([]byte, error) {
	inventory, err := InspectRoot(root, RootOptions{})
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, file := range inventory.Files {
		pathValue := filepath.Join(root, filepath.FromSlash(file.Path))
		data, err := os.ReadFile(pathValue)
		if err != nil {
			return nil, err
		}
		header := &tar.Header{
			Name:       file.Path,
			Mode:       int64(file.Mode),
			Size:       int64(len(data)),
			ModTime:    epoch,
			AccessTime: epoch,
			ChangeTime: epoch,
			Typeflag:   tar.TypeReg,
			Uid:        0,
			Gid:        0,
			Uname:      "",
			Gname:      "",
		}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func inventoryFile(pathValue, canonical string) (FileEvidence, error) {
	data, err := os.ReadFile(pathValue)
	if err != nil {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.unreadable", Path: canonical, Detail: err.Error()}
	}
	info, err := os.Lstat(pathValue)
	if err != nil {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.unreadable", Path: canonical, Detail: err.Error()}
	}
	if info.Mode()&os.ModeType != 0 || !info.Mode().IsRegular() {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.type_invalid", Path: canonical, Detail: "file must be a regular file"}
	}
	return FileEvidence{Path: canonical, Type: "regular", Size: int64(len(data)), Mode: uint32(info.Mode().Perm()), Digest: sha256Digest(data)}, nil
}

func regularFileEvidence(pathValue, canonical string) (FileEvidence, error) {
	info, err := os.Lstat(pathValue)
	if err != nil {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.unreadable", Path: canonical, Detail: err.Error()}
	}
	if info.Mode()&os.ModeType != 0 || !info.Mode().IsRegular() {
		return FileEvidence{}, &InventoryError{Code: "artifact.file.type_invalid", Path: canonical, Detail: "file must be a regular file"}
	}
	return inventoryFile(pathValue, canonical)
}

func digestInventory(files []FileEvidence) string {
	var buffer bytes.Buffer
	for _, file := range files {
		fmt.Fprintf(&buffer, "%s\x00%s\x00%d\x00%o\x00%s\n", file.Path, file.Type, file.Size, file.Mode, file.Digest)
	}
	return sha256Digest(buffer.Bytes())
}

func sha256Digest(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func sha384Digest(data []byte) string {
	digest := sha512.Sum384(data)
	return "sha384:" + hex.EncodeToString(digest[:])
}

func digestForExpected(data []byte, expected string) string {
	algorithm, _, _ := strings.Cut(expected, ":")
	if algorithm == "sha384" {
		return sha384Digest(data)
	}
	return sha256Digest(data)
}

func canonicalRelativePath(value string) (string, error) {
	canonical := filepath.ToSlash(value)
	if unsafePath(canonical) {
		return "", &InventoryError{Code: "artifact.path.unsafe", Path: canonical, Detail: "artifact path is absolute, traverses parents, or contains a link-like separator"}
	}
	return canonical, nil
}

func canonicalArchivePath(value string) (string, error) {
	if strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") {
		return "", &InventoryError{Code: "artifact.archive.path_unsafe", Path: value, Detail: "archive path is absolute or contains a backslash or NUL"}
	}
	trimmed := strings.TrimSuffix(value, "/")
	if trimmed == "" || unsafePath(trimmed) {
		return "", &InventoryError{Code: "artifact.archive.path_unsafe", Path: value, Detail: "archive path is empty, non-canonical, or traverses parents"}
	}
	return trimmed, nil
}

func ensureWithinRoot(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return &InventoryError{Code: "artifact.archive.path_escape", Path: candidate, Detail: "archive entry escapes extraction root"}
	}
	return nil
}

func normalizeExclusions(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
		if value == "" {
			continue
		}
		if unsafePath(value) {
			return nil, &InventoryError{Code: "artifact.exclusion.invalid", Path: value, Detail: "exclusion path is not canonical"}
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func excludedPath(value string, exclusions []string) bool {
	for _, exclusion := range exclusions {
		if value == exclusion || strings.HasSuffix(value, "/"+exclusion) {
			return true
		}
		if strings.HasPrefix(exclusion, "**/") && strings.HasSuffix(value, "/"+strings.TrimPrefix(exclusion, "**/")) {
			return true
		}
	}
	return false
}

func runtimeExclusions(kind ArtifactKind, policy Policy) []string {
	if kind == ArtifactSourceArchive {
		return nil
	}
	return policy.ExcludedRuntimePath
}

func requiredPaths(policy Policy, kind ArtifactKind) []string {
	if paths, exists := policy.LegalPlacement[kind]; exists && len(paths) > 0 {
		return append([]string(nil), paths...)
	}
	return append([]string(nil), policy.RequiredLegalPaths...)
}

func sbomPath(policy Policy, kind ArtifactKind, name string) (string, error) {
	candidate := "sbom/" + name + ".cdx.json"
	if prefix, exists := policy.SBOMPlacement[kind]; exists && strings.TrimSpace(prefix) != "" {
		prefix = strings.TrimSuffix(strings.ReplaceAll(prefix, "\\", "/"), "/")
		candidate = prefix + "/" + name + ".cdx.json"
	}
	return canonicalRelativePath(candidate)
}

func dependenciesForArtifact(names []string, dependencies []DependencyEvidence) []DependencyEvidence {
	if len(names) == 0 {
		return nil
	}
	selected := make([]DependencyEvidence, 0, len(names))
	for _, name := range names {
		for _, dependency := range dependencies {
			if dependency.Name == name {
				selected = append(selected, dependency)
				break
			}
		}
	}
	return selected
}

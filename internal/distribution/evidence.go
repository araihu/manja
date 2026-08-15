// Package distribution contains the evidence boundary for self-hosted
// distribution. It does not create license files, release archives, SBOMs, or
// OCI images. It only validates caller-supplied, immutable evidence and fails
// closed while authority or provenance is unresolved.
package distribution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
)

const evidenceSchemaVersion = 1

// Status is the result of an evidence gate.
type Status string

const (
	StatusPass    Status = "PASS"
	StatusBlocked Status = "BLOCKED"
)

// Scope identifies whether a dependency enters a distributed artifact.
type Scope string

const (
	ScopeShipped   Scope = "shipped"
	ScopeBuildOnly Scope = "build-only"
	ScopeTestOnly  Scope = "test-only"
)

// ArtifactKind identifies a concrete artifact whose bytes were inspected.
type ArtifactKind string

const (
	ArtifactSourceArchive ArtifactKind = "source-archive"
	ArtifactBinary        ArtifactKind = "binary-archive"
	ArtifactOCI           ArtifactKind = "oci"
	ArtifactSite          ArtifactKind = "site"
)

// Evidence is a candidate's dependency and artifact evidence. Paths and
// digests describe observed bytes; they do not grant legal authority.
type Evidence struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Subject       SubjectEvidence      `json:"subject"`
	Provenance    AuthorityEvidence    `json:"provenance"`
	RightsHolder  AuthorityEvidence    `json:"rightsHolder"`
	Legal         LegalEvidence        `json:"legal"`
	Dependencies  []DependencyEvidence `json:"dependencies"`
	Artifacts     []ArtifactEvidence   `json:"artifacts"`
}

// SubjectEvidence binds evidence to one immutable Git identity.
type SubjectEvidence struct {
	CommitSHA string `json:"commitSha"`
	TreeSHA   string `json:"treeSha"`
}

// AuthorityEvidence points at evidence for a gate. A PASS requires an
// immutable reference and digest; a BLOCKED status is valid without either.
type AuthorityEvidence struct {
	Status    Status `json:"status"`
	Reference string `json:"reference,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

// LegalEvidence describes required legal files only after authority is proved.
// The validator never synthesizes their contents or holder attribution.
type LegalEvidence struct {
	Holder     string       `json:"holder,omitempty"`
	YearRange  string       `json:"yearRange,omitempty"`
	License    FileEvidence `json:"license"`
	Notice     FileEvidence `json:"notice"`
	ThirdParty FileEvidence `json:"thirdPartyNotices"`
}

// DependencyEvidence identifies one exact dependency body or copied asset.
type DependencyEvidence struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	License   string `json:"license"`
	Scope     Scope  `json:"scope"`
	Source    string `json:"source"`
	Digest    string `json:"digest"`
}

// ArtifactEvidence identifies one actual artifact and its recursively
// inspected files. Runtime artifacts must include every required legal file.
type ArtifactEvidence struct {
	Name                     string             `json:"name"`
	Kind                     ArtifactKind       `json:"kind"`
	Source                   string             `json:"source"`
	Digest                   string             `json:"digest"`
	Inspection               InspectionEvidence `json:"inspection"`
	SBOM                     SBOMEvidence       `json:"sbom"`
	Platforms                []PlatformEvidence `json:"platforms,omitempty"`
	PlatformCoverageComplete bool               `json:"platformCoverageComplete,omitempty"`
	Dependencies             []string           `json:"dependencies"`
	Files                    []FileEvidence     `json:"files"`
}

// InspectionEvidence proves that the complete immutable artifact was scanned
// in a fresh extraction root. A caller cannot make a selected subdirectory
// authoritative by omitting this receipt.
type InspectionEvidence struct {
	Complete    bool `json:"complete"`
	FreshRoot   bool `json:"freshRoot"`
	DigestBound bool `json:"digestBound"`
}

// SBOMEvidence identifies one deterministic inventory for an artifact. The
// gate checks the receipt's identity and completeness; it does not generate,
// rewrite, or interpret an SBOM as legal advice.
type SBOMEvidence struct {
	Format   string `json:"format"`
	Source   string `json:"source"`
	Digest   string `json:"digest"`
	Complete bool   `json:"complete"`
}

// PlatformEvidence binds one OCI platform manifest to the inspected image.
type PlatformEvidence struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Digest       string `json:"digest"`
}

// FileEvidence identifies one regular file in an inspected artifact root.
type FileEvidence struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	Digest string `json:"digest"`
}

// Policy carries exclusion and required-material rules. Keep it explicit so a
// future release can review policy changes as bytes, not hidden code defaults.
type Policy struct {
	RequiredLegalPaths  []string
	ExcludedRuntimePath []string
	// LegalPlacement overrides the default root placement for an artifact
	// kind. OCI images commonly place notices below /usr/share/licenses; the
	// gate keeps that placement explicit instead of guessing it.
	LegalPlacement map[ArtifactKind][]string
	// SBOMPlacement optionally relocates an artifact's deterministic SBOM,
	// for example into an OCI image's license directory.
	SBOMPlacement map[ArtifactKind]string
}

// DefaultPolicy is the current bounded self-hosted distribution policy.
func DefaultPolicy() Policy {
	return Policy{
		RequiredLegalPaths: []string{"LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"},
		ExcludedRuntimePath: []string{
			"internal/web/static/request_composer_browser_test.go",
			"internal/web/static/schema_example_browser_test.go",
		},
		LegalPlacement: map[ArtifactKind][]string{},
		SBOMPlacement:  map[ArtifactKind]string{},
	}
}

// Finding is one fail-closed evidence violation.
type Finding struct {
	Code    string `json:"code"`
	Subject string `json:"subject,omitempty"`
	Detail  string `json:"detail"`
}

// Result is deterministic and suitable for a raw gate receipt.
type Result struct {
	Status   Status    `json:"status"`
	Findings []Finding `json:"findings"`
}

// HasCode reports whether a result includes code.
func (r Result) HasCode(code string) bool {
	for _, finding := range r.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

var (
	sha1Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hexPattern  = regexp.MustCompile(`^[0-9a-f]+$`)
)

// Evaluate validates dependency and artifact evidence. It never upgrades a
// BLOCKED authority status, even when all mechanical hashes are present.
func Evaluate(evidence Evidence, policy Policy) Result {
	findings := make([]Finding, 0)
	if evidence.SchemaVersion != evidenceSchemaVersion {
		findings = append(findings, Finding{
			Code:   "evidence.schema.unsupported",
			Detail: fmt.Sprintf("schema version %d is not supported", evidence.SchemaVersion),
		})
	}
	if !sha1Pattern.MatchString(evidence.Subject.CommitSHA) {
		findings = append(findings, Finding{Code: "subject.commit.invalid", Detail: "commit must be a full lowercase Git SHA-1"})
	}
	if !sha1Pattern.MatchString(evidence.Subject.TreeSHA) {
		findings = append(findings, Finding{Code: "subject.tree.invalid", Detail: "tree must be a full lowercase Git SHA-1"})
	}

	findings = append(findings, validateAuthority("provenance", evidence.Provenance)...)
	findings = append(findings, validateAuthority("rights_holder", evidence.RightsHolder)...)

	blockedAuthority := evidence.Provenance.Status != StatusPass || evidence.RightsHolder.Status != StatusPass
	if blockedAuthority && hasLegalClaim(evidence.Legal) {
		findings = append(findings, Finding{
			Code:   "legal.materials.before_clearance",
			Detail: "legal materials or holder attribution present while authority is not PASS",
		})
	}
	if !blockedAuthority {
		findings = append(findings, validateLegal(evidence.Legal, policy.RequiredLegalPaths)...)
	}

	dependencyByName, dependencyFindings := validateDependencies(evidence.Dependencies)
	findings = append(findings, dependencyFindings...)
	artifactNames := make(map[string]struct{}, len(evidence.Artifacts))
	for _, artifact := range evidence.Artifacts {
		if _, exists := artifactNames[artifact.Name]; exists && artifact.Name != "" {
			findings = append(findings, Finding{Code: "artifact.duplicate", Subject: artifact.Name, Detail: "artifact name is duplicated"})
		}
		artifactNames[artifact.Name] = struct{}{}
		findings = append(findings, validateArtifact(artifact, policy, dependencyByName, evidence.Legal, !blockedAuthority, true, true)...)
	}
	if len(evidence.Artifacts) == 0 {
		findings = append(findings, Finding{Code: "artifact.missing", Detail: "no produced artifact has been inspected"})
	}
	findings = append(findings, validateShippedDependencies(evidence.Dependencies, evidence.Artifacts)...)

	sortFindings(findings)
	status := StatusPass
	if len(findings) > 0 || blockedAuthority {
		status = StatusBlocked
	}
	return Result{Status: status, Findings: findings}
}

func validateAuthority(name string, authority AuthorityEvidence) []Finding {
	var findings []Finding
	switch authority.Status {
	case StatusPass:
		if authority.Reference == "" {
			findings = append(findings, Finding{Code: "authority." + name + ".reference_missing", Detail: "PASS requires an immutable evidence reference"})
		}
		if !validDigest(authority.Digest) {
			findings = append(findings, Finding{Code: "authority." + name + ".digest_invalid", Detail: "PASS requires a lowercase SHA-256 or SHA-384 digest"})
		}
	case StatusBlocked:
		findings = append(findings, Finding{Code: "authority." + name + ".blocked", Detail: "authority evidence is explicitly BLOCKED"})
	default:
		findings = append(findings, Finding{Code: "authority." + name + ".status_invalid", Detail: "authority status must be PASS or BLOCKED"})
	}
	return findings
}

func hasLegalClaim(legal LegalEvidence) bool {
	return legal.Holder != "" || legal.YearRange != "" || !isZeroFile(legal.License) || !isZeroFile(legal.Notice) || !isZeroFile(legal.ThirdParty)
}

func validateLegal(legal LegalEvidence, required []string) []Finding {
	var findings []Finding
	if legal.Holder == "" || legal.YearRange == "" {
		findings = append(findings, Finding{Code: "legal.attribution.missing", Detail: "PASS requires verified holder and evidence-backed year range"})
	}
	for _, requiredPath := range required {
		file, ok := legalFileForPath(legal, requiredPath)
		if !ok || isZeroFile(file) {
			findings = append(findings, Finding{Code: "legal.file.missing", Subject: requiredPath, Detail: "required legal file evidence is missing"})
			continue
		}
		findings = append(findings, validateFile(file, file.Path)...)
	}
	return findings
}

func legalFileForPath(legal LegalEvidence, requiredPath string) (FileEvidence, bool) {
	files := []FileEvidence{legal.License, legal.Notice, legal.ThirdParty}
	for _, file := range files {
		if file.Path == "" {
			continue
		}
		if file.Path == requiredPath || path.Base(file.Path) == path.Base(requiredPath) {
			return file, true
		}
	}
	return FileEvidence{}, false
}

func validateDependencies(dependencies []DependencyEvidence) (map[string]DependencyEvidence, []Finding) {
	byName := make(map[string]DependencyEvidence, len(dependencies))
	ecosystemByName := make(map[string]string, len(dependencies))
	var findings []Finding
	for _, dependency := range dependencies {
		key := dependency.Ecosystem + "\x00" + dependency.Name
		if _, exists := byName[key]; exists && dependency.Name != "" {
			findings = append(findings, Finding{Code: "dependency.duplicate", Subject: dependency.Name, Detail: "dependency identity is duplicated"})
		}
		byName[key] = dependency
		if prior, exists := ecosystemByName[dependency.Name]; exists && prior != dependency.Ecosystem && dependency.Name != "" {
			findings = append(findings, Finding{Code: "dependency.identity.ambiguous", Subject: dependency.Name, Detail: "artifact dependency references names only; the same name cannot occur in multiple ecosystems"})
		}
		ecosystemByName[dependency.Name] = dependency.Ecosystem
		if dependency.Ecosystem == "" || dependency.Name == "" {
			findings = append(findings, Finding{Code: "dependency.identity.missing", Subject: dependency.Name, Detail: "dependency ecosystem and name are required"})
		}
		if dependency.Version == "" || isMutableVersion(dependency.Version) {
			findings = append(findings, Finding{Code: "dependency.version.invalid", Subject: dependency.Name, Detail: "dependency version must be immutable and non-empty"})
		}
		if isUnknownLicense(dependency.License) {
			findings = append(findings, Finding{Code: "dependency.license.missing", Subject: dependency.Name, Detail: "dependency license evidence is unknown or missing"})
		}
		if dependency.Source == "" {
			findings = append(findings, Finding{Code: "dependency.source.missing", Subject: dependency.Name, Detail: "dependency source reference is required"})
		}
		if !validDigest(dependency.Digest) {
			findings = append(findings, Finding{Code: "dependency.digest.invalid", Subject: dependency.Name, Detail: "dependency requires a lowercase SHA-256 or SHA-384 digest"})
		}
		switch dependency.Scope {
		case ScopeShipped, ScopeBuildOnly, ScopeTestOnly:
		default:
			findings = append(findings, Finding{Code: "dependency.scope.invalid", Subject: dependency.Name, Detail: "dependency scope is not recognized"})
		}
	}
	return byName, findings
}

func validateArtifact(artifact ArtifactEvidence, policy Policy, dependencies map[string]DependencyEvidence, legal LegalEvidence, requireLegal, checkSBOM, checkInspection bool) []Finding {
	var findings []Finding
	if artifact.Name == "" {
		findings = append(findings, Finding{Code: "artifact.name.missing", Detail: "artifact name is required"})
	}
	if artifact.Source == "" {
		findings = append(findings, Finding{Code: "artifact.source.missing", Subject: artifact.Name, Detail: "artifact source identity is required"})
	}
	if !validDigest(artifact.Digest) {
		findings = append(findings, Finding{Code: "artifact.digest.invalid", Subject: artifact.Name, Detail: "artifact requires a lowercase SHA-256 or SHA-384 digest"})
	}
	if checkInspection && (!artifact.Inspection.Complete || !artifact.Inspection.FreshRoot || !artifact.Inspection.DigestBound) {
		findings = append(findings, Finding{Code: "artifact.inspection.incomplete", Subject: artifact.Name, Detail: "artifact must be scanned from complete digest-bound bytes in a fresh extraction root"})
	}
	if checkSBOM {
		findings = append(findings, validateSBOM(artifact.Name, artifact.SBOM)...)
	}
	switch artifact.Kind {
	case ArtifactSourceArchive, ArtifactBinary, ArtifactOCI, ArtifactSite:
	default:
		findings = append(findings, Finding{Code: "artifact.kind.invalid", Subject: artifact.Name, Detail: "artifact kind is not recognized"})
	}
	if artifact.Kind == ArtifactOCI {
		if !artifact.PlatformCoverageComplete {
			findings = append(findings, Finding{Code: "artifact.oci.coverage_incomplete", Subject: artifact.Name, Detail: "OCI evidence must attest that every published platform manifest was inspected"})
		}
		if len(artifact.Platforms) == 0 {
			findings = append(findings, Finding{Code: "artifact.oci.platforms_missing", Subject: artifact.Name, Detail: "OCI evidence must enumerate every published platform manifest"})
		}
		seenPlatforms := make(map[string]struct{}, len(artifact.Platforms))
		for _, platform := range artifact.Platforms {
			key := platform.OS + "/" + platform.Architecture
			if platform.OS == "" || platform.Architecture == "" || !validDigest(platform.Digest) {
				findings = append(findings, Finding{Code: "artifact.oci.platform_invalid", Subject: artifact.Name + ":" + key, Detail: "OCI platform requires OS, architecture, and immutable digest"})
			}
			if _, exists := seenPlatforms[key]; exists && key != "/" {
				findings = append(findings, Finding{Code: "artifact.oci.platform_duplicate", Subject: artifact.Name + ":" + key, Detail: "OCI platform is duplicated"})
			}
			seenPlatforms[key] = struct{}{}
		}
	}
	if len(artifact.Files) == 0 {
		findings = append(findings, Finding{Code: "artifact.inventory.missing", Subject: artifact.Name, Detail: "artifact has no recursively inspected file inventory"})
	}
	seenFiles := make(map[string]FileEvidence, len(artifact.Files))
	for _, file := range artifact.Files {
		if _, exists := seenFiles[file.Path]; exists && file.Path != "" {
			findings = append(findings, Finding{Code: "artifact.file.duplicate", Subject: artifact.Name + ":" + file.Path, Detail: "artifact file path is duplicated"})
		}
		if _, exists := seenFiles[file.Path]; !exists {
			seenFiles[file.Path] = file
		}
		findings = append(findings, validateFile(file, file.Path)...)
		if artifact.Kind != ArtifactSourceArchive && matchesExcluded(file.Path, policy.ExcludedRuntimePath) {
			findings = append(findings, Finding{Code: "artifact.excluded_source", Subject: artifact.Name + ":" + file.Path, Detail: "runtime artifact contains an excluded browser-test source"})
		}
	}
	if requireLegal && artifact.SBOM.Source != "" {
		file, exists := seenFiles[artifact.SBOM.Source]
		if !exists {
			findings = append(findings, Finding{Code: "artifact.sbom.placement_missing", Subject: artifact.Name + ":" + artifact.SBOM.Source, Detail: "SBOM source must be present in the final artifact inventory"})
		} else if file.Digest != artifact.SBOM.Digest {
			findings = append(findings, Finding{Code: "artifact.sbom.bytes_mismatch", Subject: artifact.Name + ":" + artifact.SBOM.Source, Detail: "SBOM receipt digest differs from the inspected artifact bytes"})
		}
	}
	seenDeps := make(map[string]struct{}, len(artifact.Dependencies))
	for _, name := range artifact.Dependencies {
		if _, exists := seenDeps[name]; exists && name != "" {
			findings = append(findings, Finding{Code: "artifact.dependency.duplicate", Subject: artifact.Name + ":" + name, Detail: "artifact dependency is duplicated"})
		}
		seenDeps[name] = struct{}{}
		dependency, exists, ambiguous := findDependency(name, dependencies)
		if ambiguous {
			findings = append(findings, Finding{Code: "artifact.dependency.ambiguous", Subject: artifact.Name + ":" + name, Detail: "artifact dependency name matches multiple ecosystems; evidence must disambiguate it"})
			continue
		}
		if !exists {
			findings = append(findings, Finding{Code: "artifact.dependency.unlisted", Subject: artifact.Name + ":" + name, Detail: "artifact dependency lacks matching evidence"})
			continue
		}
		switch dependency.Scope {
		case ScopeTestOnly:
			findings = append(findings, Finding{Code: "dependency.test_only_shipped", Subject: name, Detail: "test-only dependency appears in a produced artifact"})
		case ScopeBuildOnly:
			findings = append(findings, Finding{Code: "dependency.build_only_shipped", Subject: name, Detail: "build-only dependency appears in a produced artifact"})
		}
	}
	if requireLegal {
		for _, requiredPath := range requiredPaths(policy, artifact.Kind) {
			file, exists := seenFiles[requiredPath]
			if !exists {
				findings = append(findings, Finding{Code: "artifact.legal_file.missing", Subject: artifact.Name + ":" + requiredPath, Detail: "artifact does not contain required legal material"})
				continue
			}
			legalFile, legalExists := legalFileForPath(legal, requiredPath)
			if legalExists && (file.Size != legalFile.Size || file.Digest != legalFile.Digest) {
				findings = append(findings, Finding{Code: "artifact.legal_file.bytes_mismatch", Subject: artifact.Name + ":" + requiredPath, Detail: "legal evidence digest differs from the inspected artifact bytes"})
			}
		}
	}
	return findings
}

func validateShippedDependencies(dependencies []DependencyEvidence, artifacts []ArtifactEvidence) []Finding {
	var findings []Finding
	for _, dependency := range dependencies {
		if dependency.Scope != ScopeShipped {
			continue
		}
		bound := false
		for _, artifact := range artifacts {
			for _, name := range artifact.Dependencies {
				if name == dependency.Name {
					bound = true
					break
				}
			}
			if bound {
				break
			}
		}
		if !bound {
			findings = append(findings, Finding{Code: "dependency.shipped_unbound", Subject: dependency.Name, Detail: "shipped dependency is not bound to an inspected artifact"})
		}
	}
	return findings
}

func findDependency(name string, dependencies map[string]DependencyEvidence) (DependencyEvidence, bool, bool) {
	var match DependencyEvidence
	count := 0
	for key, dependency := range dependencies {
		if strings.HasSuffix(key, "\x00"+name) {
			match = dependency
			count++
		}
	}
	return match, count == 1, count > 1
}

func validateSBOM(artifactName string, sbom SBOMEvidence) []Finding {
	var findings []Finding
	switch sbom.Format {
	case "CycloneDX-JSON", "SPDX-JSON":
	default:
		findings = append(findings, Finding{Code: "artifact.sbom.format_invalid", Subject: artifactName, Detail: "SBOM format must be CycloneDX-JSON or SPDX-JSON"})
	}
	if sbom.Source == "" {
		findings = append(findings, Finding{Code: "artifact.sbom.source_missing", Subject: artifactName, Detail: "SBOM source identity is required"})
	}
	if !validDigest(sbom.Digest) {
		findings = append(findings, Finding{Code: "artifact.sbom.digest_invalid", Subject: artifactName, Detail: "SBOM requires a lowercase SHA-256 or SHA-384 digest"})
	}
	if !sbom.Complete {
		findings = append(findings, Finding{Code: "artifact.sbom.incomplete", Subject: artifactName, Detail: "SBOM must cover the complete inspected artifact"})
	}
	return findings
}

func validateFile(file FileEvidence, expectedPath string) []Finding {
	var findings []Finding
	if expectedPath == "" {
		findings = append(findings, Finding{Code: "artifact.path.missing", Detail: "file path is required"})
		return findings
	}
	if file.Path != expectedPath {
		findings = append(findings, Finding{Code: "artifact.path.mismatch", Subject: file.Path, Detail: "file path differs from its canonical inventory key"})
	}
	if unsafePath(file.Path) {
		findings = append(findings, Finding{Code: "artifact.path.unsafe", Subject: file.Path, Detail: "artifact path is absolute, traverses parents, or contains a link-like separator"})
	}
	if file.Type != "regular" {
		findings = append(findings, Finding{Code: "artifact.file.type_invalid", Subject: file.Path, Detail: "artifact inventory may contain regular files only; links, directories, and special files are not accepted"})
	}
	if file.Size <= 0 {
		findings = append(findings, Finding{Code: "artifact.file.size_invalid", Subject: file.Path, Detail: "file size must be positive"})
	}
	if !validDigest(file.Digest) {
		findings = append(findings, Finding{Code: "artifact.file.digest_invalid", Subject: file.Path, Detail: "file requires a lowercase SHA-256 or SHA-384 digest"})
	}
	return findings
}

func unsafePath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") {
		return true
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == ".." || part == "" {
			return true
		}
	}
	return path.Clean(value) != value
}

func matchesExcluded(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if value == pattern || strings.HasSuffix(value, "/"+pattern) {
			return true
		}
		if strings.HasPrefix(pattern, "**/") && strings.HasSuffix(value, "/"+strings.TrimPrefix(pattern, "**/")) {
			return true
		}
	}
	return false
}

func isZeroFile(file FileEvidence) bool {
	return file.Path == "" && file.Size == 0 && file.Digest == ""
}

func isUnknownLicense(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "unknown", "tbd", "todo", "unresolved":
		return true
	default:
		return false
	}
}

func isMutableVersion(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "latest", "main", "master", "head", "dev", "current":
		return true
	default:
		return false
	}
}

func validDigest(value string) bool {
	algorithm, encoded, ok := strings.Cut(value, ":")
	if !ok || !hexPattern.MatchString(encoded) {
		return false
	}
	switch algorithm {
	case "sha256":
		return len(encoded) == 64
	case "sha384":
		return len(encoded) == 96
	default:
		return false
	}
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		if findings[i].Subject != findings[j].Subject {
			return findings[i].Subject < findings[j].Subject
		}
		return findings[i].Detail < findings[j].Detail
	})
}

func deduplicateFindings(findings []Finding) []Finding {
	if len(findings) < 2 {
		return findings
	}
	sortFindings(findings)
	unique := findings[:1]
	for _, finding := range findings[1:] {
		if finding == unique[len(unique)-1] {
			continue
		}
		unique = append(unique, finding)
	}
	return unique
}

// MarshalCanonical emits stable, sorted evidence bytes. It is an evidence
// serialization seam, not a release artifact writer.
func MarshalCanonical(evidence Evidence) ([]byte, error) {
	normalized := normalize(evidence)
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal evidence: %w", err)
	}
	return append(encoded, '\n'), nil
}

// DecodeStrict decodes one evidence object and rejects unknown fields and
// trailing JSON values.
func DecodeStrict(reader io.Reader) (Evidence, error) {
	var evidence Evidence
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return Evidence{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Evidence{}, errors.New("evidence contains multiple JSON values")
		}
		return Evidence{}, fmt.Errorf("trailing evidence: %w", err)
	}
	return evidence, nil
}

func normalize(evidence Evidence) Evidence {
	copyEvidence := evidence
	copyEvidence.Dependencies = append([]DependencyEvidence(nil), evidence.Dependencies...)
	copyEvidence.Artifacts = append([]ArtifactEvidence(nil), evidence.Artifacts...)
	if copyEvidence.Dependencies == nil {
		copyEvidence.Dependencies = []DependencyEvidence{}
	}
	if copyEvidence.Artifacts == nil {
		copyEvidence.Artifacts = []ArtifactEvidence{}
	}
	for index := range copyEvidence.Artifacts {
		copyEvidence.Artifacts[index].Dependencies = append([]string(nil), evidence.Artifacts[index].Dependencies...)
		copyEvidence.Artifacts[index].Files = append([]FileEvidence(nil), evidence.Artifacts[index].Files...)
		copyEvidence.Artifacts[index].Platforms = append([]PlatformEvidence(nil), evidence.Artifacts[index].Platforms...)
		if copyEvidence.Artifacts[index].Dependencies == nil {
			copyEvidence.Artifacts[index].Dependencies = []string{}
		}
		if copyEvidence.Artifacts[index].Files == nil {
			copyEvidence.Artifacts[index].Files = []FileEvidence{}
		}
		if copyEvidence.Artifacts[index].Platforms == nil {
			copyEvidence.Artifacts[index].Platforms = []PlatformEvidence{}
		}
		sort.Strings(copyEvidence.Artifacts[index].Dependencies)
		sort.Slice(copyEvidence.Artifacts[index].Files, func(left, right int) bool {
			leftFile := copyEvidence.Artifacts[index].Files[left]
			rightFile := copyEvidence.Artifacts[index].Files[right]
			if leftFile.Path != rightFile.Path {
				return leftFile.Path < rightFile.Path
			}
			if leftFile.Type != rightFile.Type {
				return leftFile.Type < rightFile.Type
			}
			if leftFile.Size != rightFile.Size {
				return leftFile.Size < rightFile.Size
			}
			if leftFile.Mode != rightFile.Mode {
				return leftFile.Mode < rightFile.Mode
			}
			return leftFile.Digest < rightFile.Digest
		})
		sort.Slice(copyEvidence.Artifacts[index].Platforms, func(left, right int) bool {
			leftPlatform := copyEvidence.Artifacts[index].Platforms[left]
			rightPlatform := copyEvidence.Artifacts[index].Platforms[right]
			leftKey := leftPlatform.OS + "\x00" + leftPlatform.Architecture + "\x00" + leftPlatform.Digest
			rightKey := rightPlatform.OS + "\x00" + rightPlatform.Architecture + "\x00" + rightPlatform.Digest
			return leftKey < rightKey
		})
	}
	sort.Slice(copyEvidence.Dependencies, func(left, right int) bool {
		leftDependency := copyEvidence.Dependencies[left]
		rightDependency := copyEvidence.Dependencies[right]
		leftKey, _ := json.Marshal(leftDependency)
		rightKey, _ := json.Marshal(rightDependency)
		return string(leftKey) < string(rightKey)
	})
	sort.Slice(copyEvidence.Artifacts, func(left, right int) bool {
		leftArtifact := copyEvidence.Artifacts[left]
		rightArtifact := copyEvidence.Artifacts[right]
		if leftArtifact.Name != rightArtifact.Name {
			return leftArtifact.Name < rightArtifact.Name
		}
		leftKey, _ := json.Marshal(leftArtifact)
		rightKey, _ := json.Marshal(rightArtifact)
		return string(leftKey) < string(rightKey)
	})
	return copyEvidence
}

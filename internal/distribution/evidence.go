// Package distribution contains the evidence boundary for self-hosted
// distribution. It does not create license files, release archives, SBOMs, or
// OCI images. It only validates caller-supplied, immutable evidence and fails
// closed while authority or provenance is unresolved.
package distribution

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
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

// AuthorityClaims keeps first-party copyright, redistribution, and trademark
// dispositions separate. Each non-empty value must be repeated by the exact
// immutable receipt; caller-supplied claims alone never grant authority.
type AuthorityClaims struct {
	CopyrightHolder    string `json:"copyrightHolder,omitempty"`
	CopyrightYearRange string `json:"copyrightYearRange,omitempty"`
	Redistribution     string `json:"redistribution,omitempty"`
	Trademark          string `json:"trademark,omitempty"`
}

// AuthorityEvidence points at evidence for a gate. A PASS requires an
// immutable reference, tree/path/blob, exact size/mode/digest/receipt bytes,
// explicit claims, and in-process resolution; a BLOCKED status is valid
// without either.
type AuthorityEvidence struct {
	Status    Status          `json:"status"`
	Reference string          `json:"reference,omitempty"`
	Tree      string          `json:"tree,omitempty"`
	Size      int64           `json:"size,omitempty"`
	Mode      uint32          `json:"mode,omitempty"`
	Digest    string          `json:"digest,omitempty"`
	Claims    AuthorityClaims `json:"claims,omitempty"`
	// Receipt is the exact immutable receipt body named by Reference. PASS is
	// impossible unless its SHA-256/SHA-384 digest and Git blob identity both
	// match the supplied bytes.
	Receipt []byte `json:"receipt,omitempty"`

	resolved bool
}

// LicenseReceipt binds one shipped dependency license identifier to the exact
// immutable Git blob that contains the license bytes. Its resolved bit is
// intentionally not serializable: a JSON caller cannot self-assert that a
// license receipt came from the named repository revision.
type LicenseReceipt struct {
	Reference string `json:"reference,omitempty"`
	Tree      string `json:"tree,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Mode      uint32 `json:"mode,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Receipt   []byte `json:"receipt,omitempty"`

	resolved bool
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
	// LicenseReceipt is required for shipped dependencies. Build-only and
	// test-only dependencies remain outside the shipped SBOM/license gate.
	LicenseReceipt LicenseReceipt `json:"licenseReceipt,omitempty"`
	Scope          Scope          `json:"scope"`
	Source         string         `json:"source"`
	Digest         string         `json:"digest"`
}

// ArtifactEvidence identifies one actual artifact and its recursively
// inspected files. Runtime artifacts must include every required legal file.
type ArtifactEvidence struct {
	Name                        string             `json:"name"`
	Kind                        ArtifactKind       `json:"kind"`
	Source                      string             `json:"source"`
	Digest                      string             `json:"digest"`
	Inspection                  InspectionEvidence `json:"inspection"`
	SBOM                        SBOMEvidence       `json:"sbom"`
	Platforms                   []PlatformEvidence `json:"platforms,omitempty"`
	PlatformCoverageComplete    bool               `json:"platformCoverageComplete,omitempty"`
	Dependencies                []string           `json:"dependencies"`
	DependencyInventoryComplete bool               `json:"dependencyInventoryComplete"`
	Files                       []FileEvidence     `json:"files"`
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
	Size     int64  `json:"size"`
	Mode     uint32 `json:"mode"`
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
	sha1Pattern               = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hexPattern                = regexp.MustCompile(`^[0-9a-f]+$`)
	spdxIdentifierPattern     = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
	authorityReferencePattern = regexp.MustCompile(`^git:([^@\s]+)@([0-9a-f]{40}):([^#\s]+)#tree=([0-9a-f]{40})&blob=([0-9a-f]{40})$`)
	immutableVersionPattern   = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	immutableSourcePattern    = regexp.MustCompile(`^(?:https?://|git:)[^@\s]+@[0-9a-f]{40}(?:$|[/:?#][^\s]*)`)
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

	provenanceFindings := validateAuthority("provenance", evidence.Provenance, evidence.Legal)
	rightsHolderFindings := validateAuthority("rights_holder", evidence.RightsHolder, evidence.Legal)
	findings = append(findings, provenanceFindings...)
	findings = append(findings, rightsHolderFindings...)

	blockedAuthority := evidence.Provenance.Status != StatusPass || evidence.RightsHolder.Status != StatusPass || len(provenanceFindings) > 0 || len(rightsHolderFindings) > 0
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

type parsedAuthorityReference struct {
	repository string
	commit     string
	path       string
	tree       string
	blob       string
}

func parseAuthorityReference(value string) (parsedAuthorityReference, bool) {
	matches := authorityReferencePattern.FindStringSubmatch(value)
	if matches == nil || unsafePath(matches[3]) {
		return parsedAuthorityReference{}, false
	}
	return parsedAuthorityReference{repository: matches[1], commit: matches[2], path: matches[3], tree: matches[4], blob: matches[5]}, true
}

func validateAuthority(name string, authority AuthorityEvidence, legal LegalEvidence) []Finding {
	var findings []Finding
	switch authority.Status {
	case StatusPass:
		reference, referenceValid := parseAuthorityReference(authority.Reference)
		if !referenceValid {
			findings = append(findings, Finding{Code: "authority." + name + ".reference_invalid", Detail: "PASS requires a canonical Git receipt reference with immutable commit, tree, path, and blob"})
		} else if authority.Tree != reference.tree {
			findings = append(findings, Finding{Code: "authority." + name + ".tree_mismatch", Detail: "authority tree differs from the immutable receipt reference"})
		}
		if !sha1Pattern.MatchString(authority.Tree) {
			findings = append(findings, Finding{Code: "authority." + name + ".tree_invalid", Detail: "PASS requires the full lowercase Git tree SHA-1"})
		}
		if authority.Size <= 0 {
			findings = append(findings, Finding{Code: "authority." + name + ".size_invalid", Detail: "PASS requires the exact positive receipt byte size"})
		}
		if authority.Mode != 0o644 && authority.Mode != 0o755 {
			findings = append(findings, Finding{Code: "authority." + name + ".mode_invalid", Detail: "PASS requires an explicit 0644 or 0755 receipt mode"})
		}
		if !validDigest(authority.Digest) {
			findings = append(findings, Finding{Code: "authority." + name + ".digest_invalid", Detail: "PASS requires a lowercase SHA-256 or SHA-384 digest"})
		}
		if len(authority.Receipt) == 0 {
			findings = append(findings, Finding{Code: "authority." + name + ".receipt_missing", Detail: "PASS requires the exact immutable receipt bytes"})
		} else {
			if authority.Size != int64(len(authority.Receipt)) {
				findings = append(findings, Finding{Code: "authority." + name + ".receipt_size_mismatch", Detail: "receipt bytes do not match the supplied size"})
			}
			if validDigest(authority.Digest) && digestForExpected(authority.Receipt, authority.Digest) != authority.Digest {
				findings = append(findings, Finding{Code: "authority." + name + ".receipt_digest_mismatch", Detail: "receipt bytes do not match the supplied digest"})
			}
		}
		if referenceValid && len(authority.Receipt) > 0 && gitBlobSHA1(authority.Receipt) != reference.blob {
			findings = append(findings, Finding{Code: "authority." + name + ".receipt_blob_mismatch", Detail: "receipt bytes do not match the Git blob in the immutable reference"})
		}
		claims := []struct {
			label string
			value string
		}{
			{label: "copyright-holder", value: authority.Claims.CopyrightHolder},
			{label: "copyright-year-range", value: authority.Claims.CopyrightYearRange},
			{label: "redistribution", value: authority.Claims.Redistribution},
			{label: "trademark", value: authority.Claims.Trademark},
		}
		for _, claim := range claims {
			if claim.value == "" || strings.ContainsAny(claim.value, "\r\n") {
				findings = append(findings, Finding{Code: "authority." + name + ".claims_missing", Subject: claim.label, Detail: "PASS requires an explicit, single-line authority claim for copyright, redistribution, and trademark disposition"})
				continue
			}
			if !receiptContainsClaim(authority.Receipt, claim.label, claim.value) {
				findings = append(findings, Finding{Code: "authority." + name + ".claim_mismatch", Subject: claim.label, Detail: "immutable authority receipt does not contain the exact supplied claim"})
			}
		}
		if name == "rights_holder" && legal.Holder != "" && legal.YearRange != "" &&
			(authority.Claims.CopyrightHolder != legal.Holder || authority.Claims.CopyrightYearRange != legal.YearRange) {
			findings = append(findings, Finding{Code: "authority." + name + ".legal_claim_mismatch", Detail: "rights-holder claims do not authorize the supplied legal holder and year range"})
		}
		if !authority.resolved {
			findings = append(findings, Finding{Code: "authority." + name + ".unresolved", Detail: "PASS requires a receipt resolved from the immutable reference, not a serialized caller assertion"})
		}
	case StatusBlocked:
		findings = append(findings, Finding{Code: "authority." + name + ".blocked", Detail: "authority evidence is explicitly BLOCKED"})
	default:
		findings = append(findings, Finding{Code: "authority." + name + ".status_invalid", Detail: "authority status must be PASS or BLOCKED"})
	}
	return findings
}

func receiptContainsClaim(receipt []byte, label, value string) bool {
	return bytes.Contains(receipt, []byte(label+": "+value))
}

func gitBlobSHA1(data []byte) string {
	hash := sha1.New()
	_, _ = fmt.Fprintf(hash, "blob %d\x00", len(data))
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
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
		} else if !validSPDXExpression(dependency.License) {
			findings = append(findings, Finding{Code: "dependency.license.invalid", Subject: dependency.Name, Detail: "dependency license must be a valid SPDX identifier or expression"})
		}
		if dependency.Scope == ScopeShipped {
			findings = append(findings, validateLicenseReceipt(dependency.Name, dependency.License, dependency.LicenseReceipt)...)
		}
		if dependency.Source == "" {
			findings = append(findings, Finding{Code: "dependency.source.missing", Subject: dependency.Name, Detail: "dependency source reference is required"})
		} else if isMutableSource(dependency.Source) {
			findings = append(findings, Finding{Code: "dependency.source.invalid", Subject: dependency.Name, Detail: "dependency source must include an immutable commit identity"})
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
	} else if isMutableSource(artifact.Source) {
		findings = append(findings, Finding{Code: "artifact.source.invalid", Subject: artifact.Name, Detail: "artifact source must include an immutable commit identity"})
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
	if !artifact.DependencyInventoryComplete {
		findings = append(findings, Finding{Code: "artifact.dependency.inventory_incomplete", Subject: artifact.Name, Detail: "artifact requires a reviewed complete dependency closure, including an explicit empty closure when applicable"})
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
		} else if file.Size != artifact.SBOM.Size || file.Mode != artifact.SBOM.Mode {
			findings = append(findings, Finding{Code: "artifact.sbom.mode_mismatch", Subject: artifact.Name + ":" + artifact.SBOM.Source, Detail: "SBOM file size or mode differs from its evidence"})
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
			if legalExists && (file.Size != legalFile.Size || file.Mode != legalFile.Mode || file.Digest != legalFile.Digest) {
				if file.Mode != legalFile.Mode && file.Size == legalFile.Size && file.Digest == legalFile.Digest {
					findings = append(findings, Finding{Code: "artifact.legal_file.mode_mismatch", Subject: artifact.Name + ":" + requiredPath, Detail: "legal evidence mode differs from the inspected artifact bytes"})
					continue
				}
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
	if sbom.Size <= 0 {
		findings = append(findings, Finding{Code: "artifact.sbom.size_invalid", Subject: artifactName, Detail: "SBOM requires its exact positive byte size"})
	}
	if sbom.Mode != 0o644 {
		findings = append(findings, Finding{Code: "artifact.sbom.mode_invalid", Subject: artifactName, Detail: "SBOM requires an explicit 0644 file mode"})
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
	if file.Mode == 0 || file.Mode&^uint32(0o777) != 0 {
		findings = append(findings, Finding{Code: "artifact.file.mode_invalid", Subject: file.Path, Detail: "file mode must contain explicit portable permission bits"})
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

var knownSPDXIdentifiers = map[string]struct{}{
	"0BSD": {}, "AGPL-1.0-only": {}, "AGPL-1.0-or-later": {},
	"AGPL-3.0-only": {}, "AGPL-3.0-or-later": {}, "Apache-1.1": {},
	"Apache-2.0": {}, "Artistic-2.0": {}, "BSD-2-Clause": {},
	"BSD-3-Clause": {}, "BSD-3-Clause-Clear": {}, "BSD-4-Clause": {},
	"BSD-Source-Code": {}, "BSL-1.1": {}, "CC-BY-4.0": {},
	"CC-BY-SA-4.0": {}, "CC0-1.0": {}, "CDDL-1.0": {}, "EPL-1.0": {},
	"EPL-2.0": {}, "GPL-2.0": {}, "GPL-2.0-only": {},
	"GPL-2.0-or-later": {}, "GPL-3.0": {}, "GPL-3.0-only": {},
	"GPL-3.0-or-later": {}, "ISC": {}, "LGPL-2.0-only": {},
	"LGPL-2.0-or-later": {}, "LGPL-2.1": {}, "LGPL-2.1-only": {},
	"LGPL-2.1-or-later": {}, "LGPL-3.0": {}, "LGPL-3.0-only": {},
	"LGPL-3.0-or-later": {}, "MIT": {}, "MIT-0": {}, "MPL-1.1": {},
	"MPL-2.0": {}, "MPL-2.0-no-copyleft-exception": {}, "NCSA": {},
	"OFL-1.1": {}, "OpenSSL": {}, "Python-2.0": {}, "Ruby": {},
	"Unlicense": {}, "UPL-1.0": {}, "WTFPL": {}, "Zlib": {},
}

var knownSPDXExceptions = map[string]struct{}{
	"Autoconf-exception-2.0": {}, "Autoconf-exception-3.0": {},
	"Bison-exception-2.2": {}, "Classpath-exception-2.0": {},
	"CLISP-exception-2.0": {}, "FLTK-exception": {}, "Font-exception-2.0": {},
	"GCC-exception-2.0": {}, "GCC-exception-3.1": {}, "GNAT-exception": {},
	"LLVM-exception": {}, "Libtool-exception": {}, "Linux-syscall-note": {},
	"mif-exception": {}, "Nokia-Qt-exception-1.1": {},
	"OCaml-LGPL-linking-exception": {}, "OpenJDK-assembly-exception-1.0": {},
	"PS-or-PDF-font-exception-20170817": {}, "Swift-exception": {},
}

func validSPDXExpression(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	tokens := strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ").Replace(value))
	parser := spdxExpressionParser{tokens: tokens}
	if !parser.parseExpression() {
		return false
	}
	return parser.position == len(tokens)
}

type spdxExpressionParser struct {
	tokens   []string
	position int
}

func (parser *spdxExpressionParser) parseExpression() bool {
	return parser.parseOr()
}

func (parser *spdxExpressionParser) parseOr() bool {
	if !parser.parseAnd() {
		return false
	}
	for parser.position < len(parser.tokens) && parser.tokens[parser.position] == "OR" {
		parser.position++
		if !parser.parseAnd() {
			return false
		}
	}
	return true
}

func (parser *spdxExpressionParser) parseAnd() bool {
	if !parser.parseWith() {
		return false
	}
	for parser.position < len(parser.tokens) && parser.tokens[parser.position] == "AND" {
		parser.position++
		if !parser.parseWith() {
			return false
		}
	}
	return true
}

func (parser *spdxExpressionParser) parseWith() bool {
	if !parser.parsePrimary() {
		return false
	}
	if parser.position < len(parser.tokens) && parser.tokens[parser.position] == "WITH" {
		parser.position++
		if parser.position >= len(parser.tokens) || !validSPDXException(parser.tokens[parser.position]) {
			return false
		}
		parser.position++
	}
	return true
}

func (parser *spdxExpressionParser) parsePrimary() bool {
	if parser.position >= len(parser.tokens) {
		return false
	}
	token := parser.tokens[parser.position]
	if token == "(" {
		parser.position++
		if !parser.parseExpression() || parser.position >= len(parser.tokens) || parser.tokens[parser.position] != ")" {
			return false
		}
		parser.position++
		return true
	}
	if !validSPDXIdentifier(token) {
		return false
	}
	parser.position++
	return true
}

func validSPDXIdentifier(value string) bool {
	if strings.HasPrefix(value, "LicenseRef-") && len(value) > len("LicenseRef-") {
		return spdxIdentifierPattern.MatchString(strings.TrimPrefix(value, "LicenseRef-"))
	}
	_, ok := knownSPDXIdentifiers[value]
	return ok
}

func validSPDXException(value string) bool {
	if strings.HasPrefix(value, "LicenseRef-") && len(value) > len("LicenseRef-") {
		return spdxIdentifierPattern.MatchString(strings.TrimPrefix(value, "LicenseRef-"))
	}
	_, ok := knownSPDXExceptions[value]
	return ok
}

func validateLicenseReceipt(name, license string, receipt LicenseReceipt) []Finding {
	var findings []Finding
	parsed, ok := parseAuthorityReference(receipt.Reference)
	if !ok {
		findings = append(findings, Finding{Code: "dependency.license.reference_invalid", Subject: name, Detail: "shipped dependency license requires an immutable Git reference with tree, path, and blob"})
	} else if receipt.Tree != parsed.tree {
		findings = append(findings, Finding{Code: "dependency.license.tree_mismatch", Subject: name, Detail: "license receipt tree differs from the immutable reference"})
	}
	if !sha1Pattern.MatchString(receipt.Tree) {
		findings = append(findings, Finding{Code: "dependency.license.tree_invalid", Subject: name, Detail: "license receipt requires the full lowercase Git tree SHA-1"})
	}
	if receipt.Size <= 0 {
		findings = append(findings, Finding{Code: "dependency.license.size_invalid", Subject: name, Detail: "license receipt requires a positive byte size"})
	}
	if receipt.Mode != 0o644 && receipt.Mode != 0o755 {
		findings = append(findings, Finding{Code: "dependency.license.mode_invalid", Subject: name, Detail: "license receipt requires an explicit 0644 or 0755 mode"})
	}
	if !validDigest(receipt.Digest) {
		findings = append(findings, Finding{Code: "dependency.license.digest_invalid", Subject: name, Detail: "license receipt requires a lowercase SHA-256 or SHA-384 digest"})
	}
	if len(receipt.Receipt) == 0 {
		findings = append(findings, Finding{Code: "dependency.license.receipt_missing", Subject: name, Detail: "license receipt requires exact immutable bytes"})
	} else {
		if receipt.Size != int64(len(receipt.Receipt)) {
			findings = append(findings, Finding{Code: "dependency.license.size_mismatch", Subject: name, Detail: "license receipt size differs from its bytes"})
		}
		if validDigest(receipt.Digest) && digestForExpected(receipt.Receipt, receipt.Digest) != receipt.Digest {
			findings = append(findings, Finding{Code: "dependency.license.digest_mismatch", Subject: name, Detail: "license receipt bytes differ from its digest"})
		}
		if ok && gitBlobSHA1(receipt.Receipt) != parsed.blob {
			findings = append(findings, Finding{Code: "dependency.license.blob_mismatch", Subject: name, Detail: "license receipt bytes differ from the referenced Git blob"})
		}
		if !licenseReceiptAuthorizes(license, receipt.Receipt) {
			findings = append(findings, Finding{Code: "dependency.license.claim_mismatch", Subject: name, Detail: "license receipt bytes do not authorize the supplied SPDX license"})
		}
	}
	if !receipt.resolved {
		findings = append(findings, Finding{Code: "dependency.license.unresolved", Subject: name, Detail: "license receipt must be resolved from its immutable checkout"})
	}
	return findings
}

func licenseReceiptAuthorizes(expression string, receipt []byte) bool {
	text := strings.ToLower(string(receipt))
	identifiers := spdxLicenseIdentifiers(expression)
	for _, rawIdentifier := range identifiers {
		identifier := strings.ToLower(rawIdentifier)
		if strings.Contains(text, "spdx-license-identifier: "+identifier) {
			continue
		}
		if strings.HasPrefix(identifier, "licenseref-") {
			if !strings.Contains(text, "license-ref: "+identifier) {
				return false
			}
			continue
		}
		markers, known := spdxLicenseMarkers[identifier]
		if !known || !containsAll(text, markers) {
			return false
		}
	}
	return len(identifiers) > 0
}

var spdxLicenseMarkers = map[string][]string{
	"0bsd":              {"bsd zero clause"},
	"agpl-1.0-only":     {"gnu affero general public license", "version 1"},
	"agpl-1.0-or-later": {"gnu affero general public license", "version 1"},
	"agpl-3.0-only":     {"gnu affero general public license", "version 3"},
	"agpl-3.0-or-later": {"gnu affero general public license", "version 3"},
	"apache-1.1":        {"apache license", "1.1"},
	"apache-2.0":        {"apache license", "2.0"},
	"artistic-2.0":      {"artistic license", "2.0"},
	"bsd-2-clause":      {"redistribution and use in source and binary forms", "provided that"},
	"bsd-3-clause":      {"redistribution and use in source and binary forms", "endorse or promote"},
	"bsl-1.1":           {"business source license", "1.1"},
	"cc0-1.0":           {"creative commons", "cc0"},
	"cddl-1.0":          {"common development and distribution license", "1.0"},
	"epl-1.0":           {"eclipse public license", "1.0"},
	"epl-2.0":           {"eclipse public license", "2.0"},
	"gpl-2.0":           {"gnu general public license", "version 2"},
	"gpl-2.0-only":      {"gnu general public license", "version 2"},
	"gpl-2.0-or-later":  {"gnu general public license", "version 2"},
	"gpl-3.0":           {"gnu general public license", "version 3"},
	"gpl-3.0-only":      {"gnu general public license", "version 3"},
	"gpl-3.0-or-later":  {"gnu general public license", "version 3"},
	"isc":               {"isc license"},
	"lgpl-2.1-only":     {"gnu lesser general public license", "version 2.1"},
	"lgpl-2.1-or-later": {"gnu lesser general public license", "version 2.1"},
	"lgpl-3.0-only":     {"gnu lesser general public license", "version 3"},
	"lgpl-3.0-or-later": {"gnu lesser general public license", "version 3"},
	"mit":               {"mit license", "permission is hereby granted"},
	"mit-0":             {"mit no attribution"},
	"mpl-1.1":           {"mozilla public license", "1.1"},
	"mpl-2.0":           {"mozilla public license", "2.0"},
	"openssl":           {"openssl license", "redistribution"},
	"ofl-1.1":           {"sil open font license", "1.1"},
	"python-2.0":        {"python software foundation license", "2.0"},
	"ruby":              {"ruby license"},
	"unlicense":         {"unlicense", "public domain"},
	"upl-1.0":           {"universal permissive license", "1.0"},
	"wtfpl":             {"do what the fuck you want to public license"},
	"zlib":              {"zlib license", "altered source"},
}

func spdxLicenseIdentifiers(expression string) []string {
	tokens := strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(expression))
	identifiers := make([]string, 0, len(tokens))
	for index := 0; index < len(tokens); index++ {
		switch tokens[index] {
		case "AND", "OR":
			continue
		case "WITH":
			index++
			continue
		default:
			identifiers = append(identifiers, tokens[index])
		}
	}
	return identifiers
}

func containsAll(text string, markers []string) bool {
	for _, marker := range markers {
		if !strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func isMutableVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !immutableVersionPattern.MatchString(value) {
		return true
	}
	return false
}

func isMutableSource(value string) bool {
	return !immutableSourcePattern.MatchString(strings.TrimSpace(value))
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

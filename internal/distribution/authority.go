package distribution

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveAuthority reads the receipt named by an immutable Git reference from
// a caller-owned checkout and binds its exact bytes to both the declared
// digest and Git blob identity. JSON-decoded evidence never carries the
// unexported resolved bit and therefore cannot self-assert PASS.
func ResolveAuthority(root string, authority AuthorityEvidence) (AuthorityEvidence, error) {
	if authority.Status != StatusPass {
		return AuthorityEvidence{}, fmt.Errorf("authority status must be PASS before resolution")
	}
	reference, ok := parseAuthorityReference(authority.Reference)
	if !ok {
		return AuthorityEvidence{}, fmt.Errorf("authority reference is not an immutable Git receipt")
	}
	if !validDigest(authority.Digest) {
		return AuthorityEvidence{}, fmt.Errorf("authority digest is invalid")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return AuthorityEvidence{}, fmt.Errorf("inspect authority root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return AuthorityEvidence{}, fmt.Errorf("authority root must be a real directory")
	}
	receipt, err := resolveGitBlob(root, reference)
	if err != nil {
		return AuthorityEvidence{}, fmt.Errorf("resolve authority receipt: %w", err)
	}
	if len(authority.Receipt) > 0 && !bytes.Equal(authority.Receipt, receipt) {
		return AuthorityEvidence{}, fmt.Errorf("supplied authority receipt bytes differ from referenced bytes")
	}
	if digestForExpected(receipt, authority.Digest) != authority.Digest {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt digest does not match referenced bytes")
	}
	if gitBlobSHA1(receipt) != reference.blob {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt Git blob does not match referenced bytes")
	}
	authority.Receipt = append([]byte(nil), receipt...)
	authority.resolved = true
	return authority, nil
}

func ResolveDependencyLicense(root string, dependency DependencyEvidence) (DependencyEvidence, error) {
	if dependency.Scope != ScopeShipped {
		return DependencyEvidence{}, fmt.Errorf("dependency license resolution requires shipped scope")
	}
	if !validSPDXExpression(dependency.License) {
		return DependencyEvidence{}, fmt.Errorf("dependency license is not a valid SPDX identifier or expression")
	}
	reference, ok := parseAuthorityReference(dependency.LicenseReceipt.Reference)
	if !ok {
		return DependencyEvidence{}, fmt.Errorf("dependency license reference is not an immutable Git receipt")
	}
	if !validDigest(dependency.LicenseReceipt.Digest) {
		return DependencyEvidence{}, fmt.Errorf("dependency license digest is invalid")
	}
	receipt, mode, err := resolveGitBlobWithMode(root, reference)
	if err != nil {
		return DependencyEvidence{}, fmt.Errorf("resolve dependency license: %w", err)
	}
	if len(dependency.LicenseReceipt.Receipt) > 0 && !bytes.Equal(dependency.LicenseReceipt.Receipt, receipt) {
		return DependencyEvidence{}, fmt.Errorf("supplied dependency license bytes differ from referenced bytes")
	}
	if dependency.LicenseReceipt.Size != int64(len(receipt)) {
		return DependencyEvidence{}, fmt.Errorf("dependency license size differs from referenced bytes")
	}
	if dependency.LicenseReceipt.Mode != mode {
		return DependencyEvidence{}, fmt.Errorf("dependency license mode differs from referenced Git mode")
	}
	if digestForExpected(receipt, dependency.LicenseReceipt.Digest) != dependency.LicenseReceipt.Digest {
		return DependencyEvidence{}, fmt.Errorf("dependency license digest does not match referenced bytes")
	}
	dependency.LicenseReceipt.Receipt = append([]byte(nil), receipt...)
	dependency.LicenseReceipt.resolved = true
	return dependency, nil
}

func resolveGitBlob(root string, reference parsedAuthorityReference) ([]byte, error) {
	receipt, _, err := resolveGitBlobWithMode(root, reference)
	return receipt, err
}

func resolveGitBlobWithMode(root string, reference parsedAuthorityReference) ([]byte, uint32, error) {
	if strings.TrimSpace(root) == "" {
		return nil, 0, fmt.Errorf("Git checkout root is required")
	}
	inside, err := gitOutput(root, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return nil, 0, fmt.Errorf("root is not a non-bare Git worktree")
	}
	remote, err := gitOutput(root, "remote", "get-url", "origin")
	if err != nil || canonicalRepository(remote) != canonicalRepository(reference.repository) {
		return nil, 0, fmt.Errorf("Git origin does not match immutable repository reference")
	}
	commitType, err := gitOutput(root, "cat-file", "-t", reference.commit)
	if err != nil || strings.TrimSpace(commitType) != "commit" {
		return nil, 0, fmt.Errorf("immutable commit is absent from the checkout")
	}
	object, err := gitOutput(root, "rev-parse", "--verify", reference.commit+":"+reference.path)
	if err != nil || strings.TrimSpace(object) != reference.blob {
		return nil, 0, fmt.Errorf("immutable commit path does not resolve to the referenced blob")
	}
	objectType, err := gitOutput(root, "cat-file", "-t", reference.blob)
	if err != nil || strings.TrimSpace(objectType) != "blob" {
		return nil, 0, fmt.Errorf("referenced Git object is not a regular-file blob")
	}
	modeOutput, err := gitOutput(root, "ls-tree", reference.commit, "--", reference.path)
	if err != nil {
		return nil, 0, fmt.Errorf("read immutable Git file mode: %w", err)
	}
	fields := strings.Fields(modeOutput)
	if len(fields) < 3 || fields[1] != "blob" || fields[2] != reference.blob {
		return nil, 0, fmt.Errorf("immutable Git path mode is not a regular file")
	}
	mode := uint32(0)
	switch fields[0] {
	case "100644":
		mode = 0o644
	case "100755":
		mode = 0o755
	default:
		return nil, 0, fmt.Errorf("immutable Git path has unsupported mode %s", fields[0])
	}
	receipt, err := gitOutputBytes(root, "cat-file", "blob", reference.blob)
	if err != nil {
		return nil, 0, fmt.Errorf("read immutable Git blob: %w", err)
	}
	return receipt, mode, nil
}

func gitOutput(root string, args ...string) (string, error) {
	return stringOutput(root, args...)
}

func stringOutput(root string, args ...string) (string, error) {
	output, err := gitCommand(root, args...).Output()
	return string(output), err
}

func gitOutputBytes(root string, args ...string) ([]byte, error) {
	return gitCommand(root, args...).Output()
}

func gitCommand(root string, args ...string) *exec.Cmd {
	commandArgs := append([]string{"-C", root}, args...)
	return exec.Command("git", commandArgs...)
}

func canonicalRepository(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "ssh://")
	value = strings.TrimPrefix(value, "git://")
	value = strings.TrimPrefix(value, "git@")
	if host, pathValue, ok := strings.Cut(value, ":"); ok && !strings.Contains(host, "/") {
		value = host + "/" + pathValue
	}
	return strings.Trim(value, "/")
}

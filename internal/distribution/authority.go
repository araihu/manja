package distribution

import (
	"bytes"
	"fmt"
	"os"
)

// ResolveAuthority reads the receipt named by an immutable Git reference from
// a caller-owned checkout and binds its exact bytes to both the declared
// digest and Git blob identity. JSON-decoded evidence never carries the
// unexported resolved bit and therefore cannot self-assert PASS.
func ResolveAuthority(root string, authority AuthorityEvidence) (AuthorityEvidence, error) {
	if authority.Status != StatusPass {
		return AuthorityEvidence{}, fmt.Errorf("authority status must be PASS before resolution")
	}
	matches := authorityReferencePattern.FindStringSubmatch(authority.Reference)
	if matches == nil || unsafePath(matches[2]) {
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
	pathValue, err := containedPath(root, matches[2])
	if err != nil {
		return AuthorityEvidence{}, fmt.Errorf("contain authority receipt: %w", err)
	}
	info, err := os.Lstat(pathValue)
	if err != nil {
		return AuthorityEvidence{}, fmt.Errorf("read authority receipt: %w", err)
	}
	if info.Mode()&os.ModeType != 0 || !info.Mode().IsRegular() {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt must be a regular file")
	}
	receipt, err := os.ReadFile(pathValue)
	if err != nil {
		return AuthorityEvidence{}, fmt.Errorf("read authority receipt: %w", err)
	}
	if len(authority.Receipt) > 0 && !bytes.Equal(authority.Receipt, receipt) {
		return AuthorityEvidence{}, fmt.Errorf("supplied authority receipt bytes differ from referenced bytes")
	}
	if digestForExpected(receipt, authority.Digest) != authority.Digest {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt digest does not match referenced bytes")
	}
	if gitBlobSHA1(receipt) != matches[3] {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt Git blob does not match referenced bytes")
	}
	authority.Receipt = append([]byte(nil), receipt...)
	authority.resolved = true
	return authority, nil
}

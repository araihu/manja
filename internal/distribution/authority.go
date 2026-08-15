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
	commit := matches[1]
	receiptPath := matches[2]
	blob := matches[3]
	if err := verifyGitReceipt(root, commit, receiptPath, blob); err != nil {
		return AuthorityEvidence{}, err
	}
	pathValue, err := containedPath(root, receiptPath)
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
	if gitBlobSHA1(receipt) != blob {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt Git blob does not match referenced bytes")
	}
	gitReceipt, err := runGit(root, "cat-file", "blob", commit+":"+receiptPath)
	if err != nil {
		return AuthorityEvidence{}, fmt.Errorf("read authority receipt from Git object: %w", err)
	}
	if !bytes.Equal(gitReceipt, receipt) {
		return AuthorityEvidence{}, fmt.Errorf("authority receipt bytes differ from referenced Git object")
	}
	authority.Receipt = append([]byte(nil), receipt...)
	authority.resolved = true
	return authority, nil
}

func verifyGitReceipt(root, commit, receiptPath, blob string) error {
	insideWorkTree, err := runGitText(root, "rev-parse", "--is-inside-work-tree")
	if err != nil || insideWorkTree != "true" {
		if err != nil {
			return fmt.Errorf("authority root must be a Git worktree: %w", err)
		}
		return fmt.Errorf("authority root must be a Git worktree")
	}
	resolvedCommit, err := runGitText(root, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil || resolvedCommit != commit {
		if err != nil {
			return fmt.Errorf("authority commit is not a real Git object: %w", err)
		}
		return fmt.Errorf("authority commit is not a real Git object")
	}
	if _, err := runGitText(root, "rev-parse", "--verify", commit+"^{tree}"); err != nil {
		return fmt.Errorf("authority commit tree is not a real Git object: %w", err)
	}
	resolvedBlob, err := runGitText(root, "rev-parse", "--verify", commit+":"+receiptPath)
	if err != nil || resolvedBlob != blob {
		if err != nil {
			return fmt.Errorf("authority receipt path is not present in the referenced Git tree: %w", err)
		}
		return fmt.Errorf("authority receipt path does not resolve to the referenced Git blob")
	}
	objectType, err := runGitText(root, "cat-file", "-t", commit+":"+receiptPath)
	if err != nil || objectType != "blob" {
		if err != nil {
			return fmt.Errorf("authority receipt path is not a Git blob: %w", err)
		}
		return fmt.Errorf("authority receipt path is not a Git blob")
	}
	return nil
}

func runGitText(root string, args ...string) (string, error) {
	output, err := runGit(root, args...)
	return strings.TrimSpace(string(output)), err
}

func runGit(root string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

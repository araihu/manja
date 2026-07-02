package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/internal/core"
)

type Git struct {
	Repo     string
	Ref      string
	Path     string
	Username string
	Token    string
	// SSHPrivateKey is PEM/OpenSSH private key material used for SSH clone URLs.
	SSHPrivateKey string
}

func (g Git) Fetch(ctx context.Context) (core.SpecFile, core.Revision, error) {
	if err := ctx.Err(); err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	if g.Repo == "" {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("git source repo is required")
	}
	if g.Path == "" {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("git source spec path is required")
	}
	repo, cleanup, err := gitWorktree(ctx, g.cloneURL(), g.SSHPrivateKey)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	defer cleanup()

	ref := g.Ref
	if ref == "" {
		ref = "HEAD"
	}

	commit, err := gitOutput(ctx, repo, "rev-parse", ref)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("resolve git ref %q: %w", ref, err)
	}
	data, err := gitOutputBytes(ctx, repo, "show", commit+":"+g.Path)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: %w", g.Path, ref, err)
	}

	sum := sha256.Sum256([]byte(commit + ":" + g.Path))
	id := "git-" + hex.EncodeToString(sum[:])[:16]
	return core.SpecFile{
			SourceID: g.Repo,
			Path:     g.Path,
			Format:   specFormat(g.Path),
			Bytes:    data,
		}, core.Revision{
			ID:        id,
			SourceID:  g.Repo,
			Ref:       ref,
			CommitSHA: commit,
		}, nil
}

func (g Git) Discover(ctx context.Context) ([]core.RevisionCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g.Repo == "" {
		return nil, fmt.Errorf("git source repo is required")
	}
	repo, cleanup, err := gitWorktree(ctx, g.cloneURL(), g.SSHPrivateKey)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	out, err := gitOutput(ctx, repo, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(*objectname)", "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, fmt.Errorf("discover git refs: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	candidates := make([]core.RevisionCandidate, 0, len(lines))
	seen := map[string]bool{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		refname, rest, ok := strings.Cut(line, "\x00")
		if !ok {
			continue
		}
		commit, peeled, ok := strings.Cut(rest, "\x00")
		if !ok {
			continue
		}
		kind, ref, ok := gitCandidateRef(refname)
		if !ok {
			continue
		}
		if kind == "tag" && strings.TrimSpace(peeled) != "" {
			commit = peeled
		}
		key := kind + "\x00" + ref
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, core.RevisionCandidate{
			SourceID:  g.Repo,
			Ref:       ref,
			Kind:      kind,
			CommitSHA: strings.TrimSpace(commit),
		})
	}
	return candidates, nil
}

func gitCandidateRef(refname string) (string, string, bool) {
	refname = strings.TrimSpace(refname)
	if ref, ok := strings.CutPrefix(refname, "refs/heads/"); ok {
		return "branch", ref, ref != ""
	}
	if ref, ok := strings.CutPrefix(refname, "refs/remotes/"); ok {
		if ref == "HEAD" || strings.HasSuffix(ref, "/HEAD") {
			return "", "", false
		}
		_, branch, ok := strings.Cut(ref, "/")
		if !ok {
			return "", "", false
		}
		ref = branch
		return "branch", ref, ref != ""
	}
	if ref, ok := strings.CutPrefix(refname, "refs/tags/"); ok {
		return "tag", ref, ref != ""
	}
	return "", "", false
}

func (g Git) cloneURL() string {
	if g.Username == "" && g.Token == "" {
		return g.Repo
	}
	parsed, err := url.Parse(g.Repo)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return g.Repo
	}
	if g.Username != "" {
		parsed.User = url.UserPassword(g.Username, g.Token)
	} else {
		parsed.User = url.User(g.Token)
	}
	return parsed.String()
}

func gitWorktree(ctx context.Context, repo, sshPrivateKey string) (string, func(), error) {
	if info, err := os.Stat(repo); err == nil && info.IsDir() {
		return repo, func() {}, nil
	}

	baseDir, err := os.MkdirTemp("", "manja-git-source-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create git source checkout: %w", err)
	}
	dir := filepath.Join(baseDir, "checkout")
	cleanup := func() { _ = os.RemoveAll(baseDir) }
	env, envCleanup, err := gitSSHEnv(sshPrivateKey, baseDir)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer envCleanup()
	if _, err := gitOutputBytesRedacted(ctx, "", env, []string{"clone", "--no-checkout", "--quiet", redactURL(repo), dir}, "clone", "--no-checkout", "--quiet", repo, dir); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("clone git source %q: %w", redactURL(repo), err)
	}
	return dir, cleanup, nil
}

func gitSSHEnv(privateKey, dir string) ([]string, func(), error) {
	if privateKey == "" {
		return nil, func() {}, nil
	}
	keyPath := filepath.Join(dir, "ssh-key")
	if err := os.WriteFile(keyPath, []byte(privateKey), 0o600); err != nil {
		return nil, func() {}, fmt.Errorf("write git ssh key: %w", err)
	}
	command := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", keyPath)
	return []string{"GIT_SSH_COMMAND=" + command}, func() { _ = os.Remove(keyPath) }, nil
}

func specFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	default:
		return "yaml"
	}
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	out, err := gitOutputBytes(ctx, repo, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitOutputBytes(ctx context.Context, repo string, args ...string) ([]byte, error) {
	return gitOutputBytesRedacted(ctx, repo, nil, args, args...)
}

func gitOutputBytesRedacted(ctx context.Context, repo string, env []string, displayArgs []string, args ...string) ([]byte, error) {
	fullArgs := args
	if repo != "" {
		fullArgs = append([]string{"-C", repo}, args...)
		displayArgs = append([]string{"-C", repo}, displayArgs...)
	}
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(displayArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	if username := parsed.User.Username(); username != "" {
		parsed.User = url.UserPassword(username, "xxxxx")
	} else {
		parsed.User = url.User("xxxxx")
	}
	return parsed.String()
}

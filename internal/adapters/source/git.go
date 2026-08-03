package source

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	core "github.com/araihu/manja/domain"
)

const (
	maxGitCommandOutputBytes = uint64(8 << 20)
	maxGitDiagnosticBytes    = uint64(64 << 10)
	maxGitRepositoryBytes    = uint64(128 << 20)
)

var (
	errGitOutputLimit     = errors.New("Git command output exceeds limit")
	errGitRepositoryLimit = errors.New("Git repository exceeds disk limit")
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
	info, err := gitCommitInfo(ctx, repo, commit)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git commit info %q: %w", commit, err)
	}

	sum := sha256.Sum256([]byte(commit + ":" + g.Path))
	id := "git-" + hex.EncodeToString(sum[:])[:16]
	return core.SpecFile{
			SourceID: g.Repo,
			Path:     g.Path,
			Format:   specFormat(g.Path),
			Bytes:    data,
		}, core.Revision{
			ID:          id,
			SourceID:    g.Repo,
			Ref:         ref,
			CommitSHA:   commit,
			AuthorName:  info.AuthorName,
			AuthorEmail: info.AuthorEmail,
			Message:     info.Message,
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
		info, err := gitCommitInfo(ctx, repo, commit)
		if err != nil {
			return nil, fmt.Errorf("read git commit info %q: %w", commit, err)
		}
		key := kind + "\x00" + ref
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, core.RevisionCandidate{
			SourceID:    g.Repo,
			Ref:         ref,
			Kind:        kind,
			CommitSHA:   strings.TrimSpace(commit),
			AuthorName:  info.AuthorName,
			AuthorEmail: info.AuthorEmail,
			Message:     info.Message,
		})
	}
	return candidates, nil
}

type gitCommitDetails struct {
	AuthorName  string
	AuthorEmail string
	Message     string
}

func gitCommitInfo(ctx context.Context, repo, commit string) (gitCommitDetails, error) {
	out, err := gitOutput(ctx, repo, "show", "-s", "--format=%an%x00%ae%x00%s", commit)
	if err != nil {
		return gitCommitDetails{}, err
	}
	name, rest, ok := strings.Cut(out, "\x00")
	if !ok {
		return gitCommitDetails{Message: strings.TrimSpace(out)}, nil
	}
	email, message, ok := strings.Cut(rest, "\x00")
	if !ok {
		return gitCommitDetails{
			AuthorName: strings.TrimSpace(name),
			Message:    strings.TrimSpace(rest),
		}, nil
	}
	return gitCommitDetails{
		AuthorName:  strings.TrimSpace(name),
		AuthorEmail: strings.TrimSpace(email),
		Message:     strings.TrimSpace(message),
	}, nil
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

func gitCatalogRepository(ctx context.Context, repo, reference, sshPrivateKey string) (string, string, func(), error) {
	if info, err := os.Stat(repo); err == nil && info.IsDir() {
		return repo, reference, func() {}, nil
	}
	baseDir, err := os.MkdirTemp("", "manja-git-catalog-*")
	if err != nil {
		return "", "", func() {}, fmt.Errorf("create Git catalog checkout: %w", err)
	}
	directory := filepath.Join(baseDir, "checkout")
	cleanup := func() { _ = os.RemoveAll(baseDir) }
	env, envCleanup, err := gitSSHEnv(sshPrivateKey, baseDir)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	defer envCleanup()
	if _, err := gitOutputBytesRedactedLimit(ctx, "", env, []string{"init", "-q", directory}, maxGitDiagnosticBytes, "", "init", "-q", directory); err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("initialize Git catalog checkout: %w", err)
	}
	if _, err := gitOutputBytesRedactedLimit(
		ctx,
		directory,
		env,
		[]string{"fetch", "--quiet", "--depth=1", "--filter=blob:none", "--no-tags", redactURL(repo), reference},
		maxGitDiagnosticBytes,
		directory,
		"fetch", "--quiet", "--depth=1", "--filter=blob:none", "--no-tags", repo, reference,
	); err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("fetch Git catalog ref %q from %q: %w", reference, redactURL(repo), err)
	}
	return directory, "FETCH_HEAD", cleanup, nil
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
	return gitOutputLimit(ctx, repo, maxGitCommandOutputBytes, args...)
}

func gitOutputLimit(ctx context.Context, repo string, limit uint64, args ...string) (string, error) {
	out, err := gitOutputBytesLimit(ctx, repo, limit, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitOutputBytes(ctx context.Context, repo string, args ...string) ([]byte, error) {
	return gitOutputBytesLimit(ctx, repo, maxGitCommandOutputBytes, args...)
}

func gitOutputBytesLimit(ctx context.Context, repo string, limit uint64, args ...string) ([]byte, error) {
	return gitOutputBytesRedactedLimit(ctx, repo, nil, args, limit, "", args...)
}

func gitOutputBytesRedacted(ctx context.Context, repo string, env []string, displayArgs []string, args ...string) ([]byte, error) {
	return gitOutputBytesRedactedLimit(ctx, repo, env, displayArgs, maxGitCommandOutputBytes, "", args...)
}

func gitOutputBytesRedactedLimit(ctx context.Context, repo string, env []string, displayArgs []string, limit uint64, diskRoot string, args ...string) ([]byte, error) {
	fullArgs := args
	if repo != "" {
		fullArgs = append([]string{"-C", repo}, args...)
		displayArgs = append([]string{"-C", repo}, displayArgs...)
	}
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	stdout := &boundedGitBuffer{limit: limit}
	stderr := &boundedGitBuffer{limit: maxGitDiagnosticBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := runGitCommand(ctx, cmd, diskRoot)
	if errors.Is(stdout.err, errGitOutputLimit) || errors.Is(stderr.err, errGitOutputLimit) {
		return nil, fmt.Errorf("git %s: %w", strings.Join(displayArgs, " "), errGitOutputLimit)
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(displayArgs, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

type boundedGitBuffer struct {
	buffer bytes.Buffer
	limit  uint64
	err    error
}

func (buffer *boundedGitBuffer) Write(data []byte) (int, error) {
	if buffer.err != nil {
		return 0, buffer.err
	}
	remaining := int64(buffer.limit) - int64(buffer.buffer.Len())
	if remaining <= 0 || int64(len(data)) > remaining {
		if remaining > 0 {
			_, _ = buffer.buffer.Write(data[:int(remaining)])
		}
		buffer.err = errGitOutputLimit
		return len(data), buffer.err
	}
	return buffer.buffer.Write(data)
}

func (buffer *boundedGitBuffer) Bytes() []byte {
	return append([]byte(nil), buffer.buffer.Bytes()...)
}

func (buffer *boundedGitBuffer) String() string {
	return buffer.buffer.String()
}

func runGitCommand(ctx context.Context, command *exec.Cmd, diskRoot string) error {
	if diskRoot == "" {
		return command.Run()
	}
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			_ = command.Process.Kill()
			<-done
			return ctx.Err()
		case <-ticker.C:
			exceeded, err := directoryExceeds(diskRoot, maxGitRepositoryBytes)
			if err != nil {
				_ = command.Process.Kill()
				<-done
				return err
			}
			if exceeded {
				_ = command.Process.Kill()
				<-done
				return errGitRepositoryLimit
			}
		}
	}
}

func directoryExceeds(root string, limit uint64) (bool, error) {
	var total uint64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 0 {
			total += uint64(info.Size())
		}
		if total > limit {
			return errGitRepositoryLimit
		}
		return nil
	})
	if errors.Is(err, errGitRepositoryLimit) {
		return true, nil
	}
	return false, err
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

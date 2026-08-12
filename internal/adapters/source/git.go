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
	"strconv"
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
	ref := g.Ref
	if ref == "" {
		ref = "HEAD"
	}
	repo, resolvedRef, cleanup, err := gitSourceRepository(ctx, g.cloneURL(), ref, g.SSHPrivateKey)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	defer cleanup()

	commit, err := gitOutputLimit(ctx, repo, 128, "rev-parse", "--verify", resolvedRef+"^{commit}")
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("resolve git ref %q: %w", ref, err)
	}
	objectID, err := gitOutputBytesEnvLimit(ctx, repo, []string{"GIT_NO_LAZY_FETCH=1"}, 128, "rev-parse", "--verify", commit+":"+g.Path)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: %w", g.Path, ref, err)
	}
	object := strings.TrimSpace(string(objectID))
	if !isFullGitObjectID(object) {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: invalid object ID", g.Path, ref)
	}
	sizeBytes, err := gitOutputBytesEnvLimit(ctx, repo, []string{"GIT_NO_LAZY_FETCH=1"}, 32, "cat-file", "-s", object)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: object is unavailable or exceeds %d bytes: %w", g.Path, ref, maxGitCommandOutputBytes, err)
	}
	size, err := parseGitObjectSize(sizeBytes)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: %w", g.Path, ref, err)
	}
	if size > maxGitCommandOutputBytes {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: exceeds %d bytes", g.Path, ref, maxGitCommandOutputBytes)
	}
	data, err := gitOutputBytesEnvLimit(ctx, repo, []string{"GIT_NO_LAZY_FETCH=1"}, size+1, "cat-file", "blob", object)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: %w", g.Path, ref, err)
	}
	if uint64(len(data)) != size {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read git spec %q at %q: object changed length", g.Path, ref)
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
	repo, cleanup, err := gitDiscoveryRepository(ctx, g.cloneURL(), g.SSHPrivateKey)
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

func gitSourceRepository(ctx context.Context, repo, reference, sshPrivateKey string) (string, string, func(), error) {
	if info, err := os.Stat(repo); err == nil && info.IsDir() {
		return repo, reference, func() {}, nil
	}
	repository, cleanup, err := gitRemoteRepository(
		ctx,
		"manja-git-source-*",
		repo,
		sshPrivateKey,
		fmt.Sprintf("blob:limit=%d", maxGitCommandOutputBytes+1),
		[]string{reference},
	)
	if err != nil {
		return "", "", func() {}, fmt.Errorf("fetch git source ref %q from %q: %w", reference, redactURL(repo), err)
	}
	return repository, "FETCH_HEAD", cleanup, nil
}

func gitDiscoveryRepository(ctx context.Context, repo, sshPrivateKey string) (string, func(), error) {
	if info, err := os.Stat(repo); err == nil && info.IsDir() {
		return repo, func() {}, nil
	}
	repository, cleanup, err := gitRemoteRepository(
		ctx,
		"manja-git-discovery-*",
		repo,
		sshPrivateKey,
		"blob:none",
		[]string{"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*"},
	)
	if err != nil {
		return "", func() {}, fmt.Errorf("fetch git source refs from %q: %w", redactURL(repo), err)
	}
	return repository, cleanup, nil
}

func gitRemoteRepository(ctx context.Context, prefix, repo, sshPrivateKey, filter string, refspecs []string) (string, func(), error) {
	baseDir, err := os.MkdirTemp("", prefix)
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
	if _, err := gitOutputBytesRedactedLimit(ctx, "", env, []string{"init", "-q", dir}, maxGitDiagnosticBytes, baseDir, "init", "-q", dir); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("initialize git source checkout: %w", err)
	}
	displayArgs := []string{"fetch", "--quiet", "--depth=1", "--filter=" + filter, "--no-tags", redactURL(repo)}
	args := []string{"fetch", "--quiet", "--depth=1", "--filter=" + filter, "--no-tags", repo}
	displayArgs = append(displayArgs, refspecs...)
	args = append(args, refspecs...)
	if _, err := gitOutputBytesRedactedLimit(ctx, dir, env, displayArgs, maxGitDiagnosticBytes, dir, args...); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dir, cleanup, nil
}

func parseGitObjectSize(output []byte) (uint64, error) {
	value := strings.TrimSpace(string(output))
	size, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Git object size %q", value)
	}
	return size, nil
}

func gitCatalogRepository(ctx context.Context, repo, reference, sshPrivateKey string) (string, string, func(), error) {
	return gitCatalogRepositoryWithObjectFormat(ctx, repo, reference, sshPrivateKey, "")
}

func gitCatalogRepositoryWithObjectFormat(ctx context.Context, repo, reference, sshPrivateKey string, objectFormat gitObjectFormat) (string, string, func(), error) {
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
	initArgs := []string{"init", "-q"}
	if objectFormat != "" {
		initArgs = append(initArgs, "--object-format="+string(objectFormat))
	}
	initArgs = append(initArgs, directory)
	if _, err := gitOutputBytesRedactedLimit(ctx, "", env, initArgs, maxGitDiagnosticBytes, "", initArgs...); err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("initialize Git catalog checkout: %w", err)
	}
	if _, err := gitOutputBytesRedactedLimit(
		ctx,
		directory,
		env,
		[]string{"fetch", "--quiet", "--depth=1", fmt.Sprintf("--filter=blob:limit=%d", maxCatalogSourceFileBytes+1), "--no-tags", redactURL(repo), reference},
		maxGitDiagnosticBytes,
		directory,
		"fetch", "--quiet", "--depth=1", fmt.Sprintf("--filter=blob:limit=%d", maxCatalogSourceFileBytes+1), "--no-tags", repo, reference,
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

func gitOutputBytesEnvLimit(ctx context.Context, repo string, env []string, limit uint64, args ...string) ([]byte, error) {
	return gitOutputBytesRedactedLimit(ctx, repo, env, args, limit, "", args...)
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
	prepareGitProcess(command)
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
			exceeded, checkErr := directoryExceeds(diskRoot, maxGitRepositoryBytes)
			if checkErr != nil {
				return checkErr
			}
			if exceeded {
				return errGitRepositoryLimit
			}
			return err
		case <-ctx.Done():
			_ = killGitProcessTree(command)
			<-done
			return ctx.Err()
		case <-ticker.C:
			exceeded, err := directoryExceeds(diskRoot, maxGitRepositoryBytes)
			if err != nil {
				_ = killGitProcessTree(command)
				<-done
				return err
			}
			if exceeded {
				_ = killGitProcessTree(command)
				<-done
				return errGitRepositoryLimit
			}
		}
	}
}

func directoryExceeds(root string, limit uint64) (bool, error) {
	var total uint64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path != root && errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
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

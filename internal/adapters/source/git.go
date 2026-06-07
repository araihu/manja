package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/internal/core"
)

type Git struct {
	Repo string
	Ref  string
	Path string
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

	commit, err := gitOutput(ctx, g.Repo, "rev-parse", ref)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("resolve git ref %q: %w", ref, err)
	}
	data, err := gitOutputBytes(ctx, g.Repo, "show", commit+":"+g.Path)
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
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

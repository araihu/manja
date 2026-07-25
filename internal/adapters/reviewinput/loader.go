// Package reviewinput loads local file and Git-ref inputs for contract reviews.
package reviewinput

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/internal/core"
)

type Loader struct {
	RepoDir string
}

func (l Loader) Load(
	ctx context.Context,
	specPath string,
	locator core.ReviewInputLocator,
) (core.SpecFile, core.Revision, error) {
	hasFile := locator.File != ""
	hasGitRef := locator.GitRef != ""
	if hasFile == hasGitRef {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("review input must set exactly one of file or git ref")
	}
	if hasFile {
		return loadFile(ctx, locator.File)
	}
	return l.loadGitRef(ctx, specPath, locator.GitRef)
}

func loadFile(ctx context.Context, path string) (core.SpecFile, core.Revision, error) {
	if err := ctx.Err(); err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("read review input file %q: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return core.SpecFile{
			Path:   path,
			Format: inputFormat(path),
			Bytes:  data,
		}, core.Revision{
			ID:  fmt.Sprintf("file-%x", sum),
			Ref: "file",
		}, nil
}

func (l Loader) loadGitRef(
	ctx context.Context,
	specPath string,
	ref string,
) (core.SpecFile, core.Revision, error) {
	if unsafeGitSpecPath(specPath) {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("unsafe git spec path %q", specPath)
	}
	if strings.HasPrefix(ref, "-") {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("unsafe git ref %q", ref)
	}

	commitOutput, err := exec.CommandContext(
		ctx, "git", "-C", l.RepoDir, "rev-parse", "--verify", ref+"^{commit}",
	).CombinedOutput()
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf(
			"resolve git ref %q: %w: %s", ref, err, strings.TrimSpace(string(commitOutput)),
		)
	}
	commit := strings.TrimSpace(string(commitOutput))

	data, err := exec.CommandContext(
		ctx, "git", "-C", l.RepoDir, "show", ref+":"+filepath.ToSlash(specPath),
	).CombinedOutput()
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf(
			"read git spec %q at %q: %w: %s", specPath, ref, err, strings.TrimSpace(string(data)),
		)
	}

	return core.SpecFile{
			Path:   specPath,
			Format: inputFormat(specPath),
			Bytes:  data,
		}, core.Revision{
			ID:        commit,
			Ref:       ref,
			CommitSHA: commit,
		}, nil
}

func unsafeGitSpecPath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func inputFormat(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return "json"
	}
	return "yaml"
}

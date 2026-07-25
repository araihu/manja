// Package reviewinput loads local file and Git-ref inputs for contract reviews.
package reviewinput

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	core "github.com/araihu/manja/domain"
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

	commitOutput, diagnostic, err := gitCommandOutput(
		ctx, l.RepoDir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}",
	)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf(
			"resolve git ref %q: %w: %s", ref, err, diagnostic,
		)
	}
	if diagnostic != "" {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf("ambiguous git ref %q: %s", ref, diagnostic)
	}
	commit := strings.TrimSpace(string(commitOutput))

	data, diagnostic, err := gitCommandOutput(
		ctx, l.RepoDir, "show", commit+":"+filepath.ToSlash(specPath),
	)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf(
			"read git spec %q at %q: %w: %s", specPath, ref, err, diagnostic,
		)
	}
	if diagnostic != "" {
		return core.SpecFile{}, core.Revision{}, fmt.Errorf(
			"read git spec %q at %q: unexpected git diagnostic: %s", specPath, ref, diagnostic,
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

func gitCommandOutput(ctx context.Context, repo string, args ...string) ([]byte, string, error) {
	gitArgs := []string{
		"--no-replace-objects",
		"-c", "core.warnAmbiguousRefs=true",
		"-C", repo,
	}
	cmd := exec.CommandContext(ctx, "git", append(gitArgs, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), strings.TrimSpace(stderr.String()), err
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

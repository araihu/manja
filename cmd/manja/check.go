package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/araihu/manja/application"
	"github.com/araihu/manja/domain"
	configadapter "github.com/araihu/manja/internal/adapters/config"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	"github.com/araihu/manja/internal/adapters/reviewformat"
	"github.com/araihu/manja/internal/adapters/reviewinput"
)

var version = "dev"

type checkConfig struct {
	ConfigPath    string
	ContractID    string
	PolicyName    string
	RepoDir       string
	Target        domain.ReviewInputLocator
	Candidate     domain.ReviewInputLocator
	Release       *domain.ReviewInputLocator
	Format        string
	EvaluatedAt   time.Time
	HasEvaluation bool
}

func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := checkConfigFromArgs(args)
	if err != nil {
		return writeCheckError(stderr, err)
	}

	configured, err := configadapter.Load(cfg.ConfigPath)
	if err != nil {
		return writeCheckError(stderr, err)
	}
	contract, err := configured.Contract(cfg.ContractID)
	if err != nil {
		return writeCheckError(stderr, err)
	}
	layer, err := contract.PolicyLayer(cfg.PolicyName)
	if err != nil {
		return writeCheckError(stderr, err)
	}
	policy, err := domain.MergePolicy(layer)
	if err != nil {
		return writeCheckError(stderr, fmt.Errorf("merge policy: %w", err))
	}

	evaluatedAt := cfg.EvaluatedAt
	if !cfg.HasEvaluation {
		evaluatedAt = time.Now().UTC()
	}
	service, err := application.NewCheckService(application.CheckDependencies{
		Inputs:    reviewinput.Loader{RepoDir: cfg.RepoDir},
		Snapshots: openapiadapter.SnapshotBuilder{},
	})
	if err != nil {
		return writeCheckError(stderr, err)
	}
	report, err := service.Run(ctx, application.CheckRequest{
		ContractID:    cfg.ContractID,
		SpecPath:      contract.Spec,
		Target:        cfg.Target,
		Candidate:     cfg.Candidate,
		Release:       cfg.Release,
		Policy:        policy,
		EvaluatedAt:   evaluatedAt,
		EngineVersion: version,
	})
	if err != nil {
		return writeCheckError(stderr, err)
	}
	if err := reviewformat.Write(stdout, cfg.Format, report); err != nil {
		return writeCheckError(stderr, fmt.Errorf("write report: %w", err))
	}
	if report.Verdict == domain.VerdictFail {
		return 1
	}
	return 0
}

func checkConfigFromArgs(args []string) (checkConfig, error) {
	fs := flag.NewFlagSet("manja check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "repository contract review config")
	contractID := fs.String("contract", "", "contract id")
	policyName := fs.String("policy", "", "policy profile")
	repoDir := fs.String("repo", ".", "Git repository directory")
	targetFile := fs.String("target-file", "", "target OpenAPI file")
	targetRef := fs.String("target-ref", "", "target Git ref")
	candidateFile := fs.String("candidate-file", "", "candidate OpenAPI file")
	candidateRef := fs.String("candidate-ref", "", "candidate Git ref")
	releaseFile := fs.String("release-file", "", "release baseline OpenAPI file")
	releaseRef := fs.String("release-ref", "", "release baseline Git ref")
	format := fs.String("format", reviewformat.FormatText, "report format: text, json, or markdown")
	evaluatedAtText := fs.String("evaluated-at", "", "policy evaluation time in RFC3339")
	if err := fs.Parse(args); err != nil {
		return checkConfig{}, err
	}
	if fs.NArg() != 0 {
		return checkConfig{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*configPath) == "" {
		return checkConfig{}, fmt.Errorf("--config is required")
	}
	if strings.TrimSpace(*contractID) == "" {
		return checkConfig{}, fmt.Errorf("--contract is required")
	}

	target, err := requiredCheckLocator("target", *targetFile, *targetRef)
	if err != nil {
		return checkConfig{}, err
	}
	candidate, err := requiredCheckLocator("candidate", *candidateFile, *candidateRef)
	if err != nil {
		return checkConfig{}, err
	}
	release, err := optionalCheckLocator("release", *releaseFile, *releaseRef)
	if err != nil {
		return checkConfig{}, err
	}

	if *format != reviewformat.FormatJSON && *format != reviewformat.FormatText && *format != reviewformat.FormatMarkdown {
		return checkConfig{}, fmt.Errorf("--format must be one of json, text, markdown")
	}
	var evaluatedAt time.Time
	if *evaluatedAtText != "" {
		evaluatedAt, err = time.Parse(time.RFC3339, *evaluatedAtText)
		if err != nil {
			return checkConfig{}, fmt.Errorf("--evaluated-at must be RFC3339: %w", err)
		}
	}

	return checkConfig{
		ConfigPath:    *configPath,
		ContractID:    *contractID,
		PolicyName:    *policyName,
		RepoDir:       *repoDir,
		Target:        target,
		Candidate:     candidate,
		Release:       release,
		Format:        *format,
		EvaluatedAt:   evaluatedAt,
		HasEvaluation: *evaluatedAtText != "",
	}, nil
}

func requiredCheckLocator(role, file, ref string) (domain.ReviewInputLocator, error) {
	if strings.TrimSpace(file) != "" && strings.TrimSpace(ref) != "" {
		return domain.ReviewInputLocator{}, fmt.Errorf("%s must set exactly one of --%s-file or --%s-ref", role, role, role)
	}
	locator, present, err := checkLocator(role, file, ref)
	if err != nil {
		return domain.ReviewInputLocator{}, err
	}
	if !present {
		return domain.ReviewInputLocator{}, fmt.Errorf("%s must set exactly one of --%s-file or --%s-ref", role, role, role)
	}
	return locator, nil
}

func optionalCheckLocator(role, file, ref string) (*domain.ReviewInputLocator, error) {
	locator, present, err := checkLocator(role, file, ref)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	return &locator, nil
}

func checkLocator(role, file, ref string) (domain.ReviewInputLocator, bool, error) {
	hasFile := strings.TrimSpace(file) != ""
	hasRef := strings.TrimSpace(ref) != ""
	if hasFile && hasRef {
		return domain.ReviewInputLocator{}, false, fmt.Errorf("%s must set at most one of --%s-file or --%s-ref", role, role, role)
	}
	if hasFile {
		return domain.ReviewInputLocator{File: file}, true, nil
	}
	if hasRef {
		return domain.ReviewInputLocator{GitRef: ref}, true, nil
	}
	return domain.ReviewInputLocator{}, false, nil
}

func writeCheckError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "manja check: %s\n", strings.Join(strings.Fields(err.Error()), " "))
	return 2
}

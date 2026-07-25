package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/araihu/manja/internal/core"
)

// CheckRequest identifies the repository contract inputs for one offline review.
type CheckRequest struct {
	ContractID    string
	SpecPath      string
	Target        core.ReviewInputLocator
	Candidate     core.ReviewInputLocator
	Release       *core.ReviewInputLocator
	Policy        core.EffectivePolicy
	EvaluatedAt   time.Time
	EngineVersion string
}

// CheckService loads, snapshots, and evaluates contract review inputs.
type CheckService struct {
	Inputs    core.ReviewInputLoader
	Snapshots core.ContractSnapshotBuilder
}

// Run performs one offline contract review.
func (s CheckService) Run(ctx context.Context, request CheckRequest) (core.ReviewReport, error) {
	if err := s.validate(request); err != nil {
		return core.ReviewReport{}, err
	}

	target, err := s.loadSnapshot(ctx, request, "target", request.Target)
	if err != nil {
		return core.ReviewReport{}, err
	}
	candidate, err := s.loadSnapshot(ctx, request, "candidate", request.Candidate)
	if err != nil {
		return core.ReviewReport{}, err
	}
	var release *core.ContractSnapshot
	if request.Release != nil {
		snapshot, err := s.loadSnapshot(ctx, request, "release", *request.Release)
		if err != nil {
			return core.ReviewReport{}, err
		}
		release = &snapshot
	}

	report, err := core.EvaluateReview(core.ReviewRequest{
		ContractID:    request.ContractID,
		Target:        target,
		Candidate:     candidate,
		Release:       release,
		Policy:        request.Policy,
		EvaluatedAt:   request.EvaluatedAt,
		EngineVersion: request.EngineVersion,
	})
	if err != nil {
		return core.ReviewReport{}, fmt.Errorf("evaluate review: %w", err)
	}
	return report, nil
}

func (s CheckService) validate(request CheckRequest) error {
	if s.Inputs == nil {
		return fmt.Errorf("input loader is required")
	}
	if s.Snapshots == nil {
		return fmt.Errorf("snapshot builder is required")
	}
	if strings.TrimSpace(request.ContractID) == "" {
		return fmt.Errorf("contract id is required")
	}
	if strings.TrimSpace(request.SpecPath) == "" {
		return fmt.Errorf("spec path is required")
	}
	if err := validateCheckLocator("target", request.Target); err != nil {
		return err
	}
	if err := validateCheckLocator("candidate", request.Candidate); err != nil {
		return err
	}
	if request.Release != nil {
		if err := validateCheckLocator("release", *request.Release); err != nil {
			return err
		}
	}
	if len(request.Policy.Layers) == 0 {
		return fmt.Errorf("policy is required")
	}
	if request.EvaluatedAt.IsZero() {
		return fmt.Errorf("evaluation time is required")
	}
	if strings.TrimSpace(request.EngineVersion) == "" {
		return fmt.Errorf("engine version is required")
	}
	return nil
}

func validateCheckLocator(role string, locator core.ReviewInputLocator) error {
	hasFile := strings.TrimSpace(locator.File) != ""
	hasRef := strings.TrimSpace(locator.GitRef) != ""
	if !hasFile && !hasRef {
		return fmt.Errorf("%s locator is required", role)
	}
	if hasFile && hasRef {
		return fmt.Errorf("%s locator must set exactly one of file or git ref", role)
	}
	return nil
}

func (s CheckService) loadSnapshot(
	ctx context.Context,
	request CheckRequest,
	role string,
	locator core.ReviewInputLocator,
) (core.ContractSnapshot, error) {
	file, revision, err := s.Inputs.Load(ctx, request.SpecPath, locator)
	if err != nil {
		return core.ContractSnapshot{}, fmt.Errorf("load %s: %w", role, err)
	}
	snapshot, err := s.Snapshots.Build(ctx, request.ContractID, file, revision)
	if err != nil {
		return core.ContractSnapshot{}, fmt.Errorf("build %s: %w", role, err)
	}
	return snapshot, nil
}

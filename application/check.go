package application

import (
	"context"
	"fmt"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

// CheckRequest identifies the repository contract inputs for one offline review.
type CheckRequest struct {
	ContractID    string
	SpecPath      string
	Target        domain.ReviewInputLocator
	Candidate     domain.ReviewInputLocator
	Release       *domain.ReviewInputLocator
	Policy        domain.EffectivePolicy
	EvaluatedAt   time.Time
	EngineVersion string
}

// CheckService loads, snapshots, and evaluates contract review inputs.
type CheckService struct {
	Inputs    port.ReviewInputLoader
	Snapshots port.ContractSnapshotBuilder
}

type CheckDependencies struct {
	Inputs    port.ReviewInputLoader
	Snapshots port.ContractSnapshotBuilder
}

func NewCheckService(dependencies CheckDependencies) (*CheckService, error) {
	if dependencies.Inputs == nil {
		return nil, dependencyError("construct check service", "input loader is required")
	}
	if dependencies.Snapshots == nil {
		return nil, dependencyError("construct check service", "snapshot builder is required")
	}
	return &CheckService{Inputs: dependencies.Inputs, Snapshots: dependencies.Snapshots}, nil
}

// Run performs one offline contract review.
func (s CheckService) Run(ctx context.Context, request CheckRequest) (domain.ReviewReport, error) {
	if err := s.validate(request); err != nil {
		return domain.ReviewReport{}, err
	}

	target, err := s.loadSnapshot(ctx, request, "target", request.Target)
	if err != nil {
		return domain.ReviewReport{}, err
	}
	candidate, err := s.loadSnapshot(ctx, request, "candidate", request.Candidate)
	if err != nil {
		return domain.ReviewReport{}, err
	}
	var release *domain.ContractSnapshot
	if request.Release != nil {
		snapshot, err := s.loadSnapshot(ctx, request, "release", *request.Release)
		if err != nil {
			return domain.ReviewReport{}, err
		}
		release = &snapshot
	}

	report, err := domain.EvaluateReview(domain.ReviewRequest{
		ContractID:    request.ContractID,
		Target:        target,
		Candidate:     candidate,
		Release:       release,
		Policy:        request.Policy,
		EvaluatedAt:   request.EvaluatedAt,
		EngineVersion: request.EngineVersion,
	})
	if err != nil {
		return domain.ReviewReport{}, wrapError(ErrorEvaluation, "evaluate review", err)
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
	if err := domain.ValidateCanonicalIdentity("contract id", request.ContractID, false); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalIdentity("spec path", request.SpecPath, false); err != nil {
		return err
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
	if err := domain.ValidateCanonicalIdentity("engine version", request.EngineVersion, false); err != nil {
		return err
	}
	return nil
}

func validateCheckLocator(role string, locator domain.ReviewInputLocator) error {
	hasFile := locator.File != ""
	hasRef := locator.GitRef != ""
	if !hasFile && !hasRef {
		return fmt.Errorf("%s locator is required", role)
	}
	if hasFile && hasRef {
		return fmt.Errorf("%s locator must set exactly one of file or git ref", role)
	}
	if hasFile {
		return domain.ValidateCanonicalIdentity(role+" file", locator.File, false)
	}
	if hasRef {
		return domain.ValidateCanonicalIdentity(role+" git ref", locator.GitRef, false)
	}
	return nil
}

func (s CheckService) loadSnapshot(
	ctx context.Context,
	request CheckRequest,
	role string,
	locator domain.ReviewInputLocator,
) (domain.ContractSnapshot, error) {
	file, revision, err := s.Inputs.Load(ctx, request.SpecPath, locator)
	if err != nil {
		return domain.ContractSnapshot{}, wrapError(ErrorInput, "load "+role, err)
	}
	if err := validatePortSpecFile(file, false); err != nil {
		return domain.ContractSnapshot{}, wrapError(ErrorInput, "validate "+role+" input", err)
	}
	if err := validatePortRevision(revision, true); err != nil {
		return domain.ContractSnapshot{}, wrapError(ErrorInput, "validate "+role+" revision", err)
	}
	if revision.ContractID != "" && revision.ContractID != request.ContractID {
		return domain.ContractSnapshot{}, wrapError(ErrorInput, "validate "+role+" revision", fmt.Errorf("revision contract id does not match review contract id"))
	}
	snapshot, err := s.Snapshots.Build(ctx, request.ContractID, file, revision)
	if err != nil {
		return domain.ContractSnapshot{}, wrapError(ErrorEvaluation, "build "+role, err)
	}
	if err := domain.ValidateContractSnapshot(snapshot); err != nil {
		return domain.ContractSnapshot{}, wrapError(ErrorEvaluation, "validate "+role+" snapshot", err)
	}
	if snapshot.ContractID != request.ContractID || snapshot.RevisionID != revision.ID {
		return domain.ContractSnapshot{}, wrapError(ErrorEvaluation, "validate "+role+" snapshot", fmt.Errorf("snapshot identity does not match its review input"))
	}
	return snapshot, nil
}

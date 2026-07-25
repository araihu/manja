package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	core "github.com/araihu/manja/domain"
)

func TestNewCheckServiceRejectsMissingDependencies(t *testing.T) {
	for _, deps := range []CheckDependencies{
		{},
		{Inputs: &checkInputLoaderFake{}},
		{Snapshots: &checkSnapshotBuilderFake{}},
	} {
		if _, err := NewCheckService(deps); err == nil {
			t.Fatalf("NewCheckService(%#v) succeeded, want error", deps)
		}
	}
}

func TestCheckServiceLoadsBuildsAndEvaluatesAllInputs(t *testing.T) {
	targetLocator := core.ReviewInputLocator{File: "target.yaml"}
	candidateLocator := core.ReviewInputLocator{GitRef: "candidate"}
	releaseLocator := core.ReviewInputLocator{GitRef: "release"}
	wantOrder := []string{
		"load:target.yaml", "build:target-revision",
		"load:candidate", "build:candidate-revision",
		"load:release", "build:release-revision",
	}
	var order []string
	loader := &checkInputLoaderFake{order: &order}
	builder := &checkSnapshotBuilderFake{order: &order}
	evaluatedAt := time.Date(2026, 7, 25, 9, 0, 0, 0, time.FixedZone("test", -3*60*60))
	policy, err := core.MergePolicy(core.PolicyLayer{
		Name:                   "stable",
		Source:                 core.PolicySourceRepository,
		RequireReleaseBaseline: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := (CheckService{Inputs: loader, Snapshots: builder}).Run(context.Background(), CheckRequest{
		ContractID:    "payments",
		SpecPath:      "docs/openapi.yaml",
		Target:        targetLocator,
		Candidate:     candidateLocator,
		Release:       &releaseLocator,
		Policy:        policy,
		EvaluatedAt:   evaluatedAt,
		EngineVersion: "test-engine",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("call order = %#v, want %#v", order, wantOrder)
	}
	if !reflect.DeepEqual(loader.specPaths, []string{"docs/openapi.yaml", "docs/openapi.yaml", "docs/openapi.yaml"}) {
		t.Fatalf("loader spec paths = %#v", loader.specPaths)
	}
	if !reflect.DeepEqual(loader.locators, []core.ReviewInputLocator{targetLocator, candidateLocator, releaseLocator}) {
		t.Fatalf("loader locators = %#v", loader.locators)
	}
	if !reflect.DeepEqual(builder.contractIDs, []string{"payments", "payments", "payments"}) {
		t.Fatalf("snapshot contract ids = %#v", builder.contractIDs)
	}
	if report.ContractID != "payments" || report.EngineVersion != "test-engine" {
		t.Fatalf("report identity = %#v", report)
	}
	if !report.EvaluatedAt.Equal(evaluatedAt) {
		t.Fatalf("evaluated at = %s, want instant %s", report.EvaluatedAt, evaluatedAt)
	}
	if len(report.Comparisons) != 2 || report.Comparisons[0].Kind != core.ComparisonPullRequest || report.Comparisons[1].Kind != core.ComparisonReleaseImpact {
		t.Fatalf("comparisons = %#v", report.Comparisons)
	}
}

func TestCheckServiceOmitsReleaseInputWhenNotRequested(t *testing.T) {
	var order []string
	service := CheckService{
		Inputs:    &checkInputLoaderFake{order: &order},
		Snapshots: &checkSnapshotBuilderFake{order: &order},
	}
	policy, err := core.MergePolicy(core.PolicyLayer{Name: "stable", Source: core.PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}

	report, err := service.Run(context.Background(), CheckRequest{
		ContractID:    "payments",
		SpecPath:      "docs/openapi.yaml",
		Target:        core.ReviewInputLocator{File: "target.yaml"},
		Candidate:     core.ReviewInputLocator{File: "candidate.yaml"},
		Policy:        policy,
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		EngineVersion: "test-engine",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantOrder := []string{"load:target.yaml", "build:target-revision", "load:candidate.yaml", "build:candidate-revision"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("call order = %#v, want %#v", order, wantOrder)
	}
	if len(report.Comparisons) != 1 || report.Comparisons[0].Kind != core.ComparisonPullRequest {
		t.Fatalf("comparisons = %#v", report.Comparisons)
	}
}

func TestCheckServiceValidatesDependenciesAndRequiredFields(t *testing.T) {
	valid := CheckRequest{
		ContractID:    "payments",
		SpecPath:      "docs/openapi.yaml",
		Target:        core.ReviewInputLocator{File: "target.yaml"},
		Candidate:     core.ReviewInputLocator{File: "candidate.yaml"},
		Policy:        core.EffectivePolicy{Layers: []core.PolicyLayer{{Name: "stable", Source: core.PolicySourceRepository}}},
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		EngineVersion: "test-engine",
	}
	validService := CheckService{Inputs: &checkInputLoaderFake{}, Snapshots: &checkSnapshotBuilderFake{}}
	tests := []struct {
		name    string
		service CheckService
		mutate  func(*CheckRequest)
		want    string
	}{
		{name: "input loader", service: CheckService{Snapshots: &checkSnapshotBuilderFake{}}, want: "input loader is required"},
		{name: "snapshot builder", service: CheckService{Inputs: &checkInputLoaderFake{}}, want: "snapshot builder is required"},
		{name: "contract", service: validService, mutate: func(request *CheckRequest) { request.ContractID = "" }, want: "contract id is required"},
		{name: "spec", service: validService, mutate: func(request *CheckRequest) { request.SpecPath = "" }, want: "spec path is required"},
		{name: "target", service: validService, mutate: func(request *CheckRequest) { request.Target = core.ReviewInputLocator{} }, want: "target locator is required"},
		{name: "candidate", service: validService, mutate: func(request *CheckRequest) { request.Candidate = core.ReviewInputLocator{} }, want: "candidate locator is required"},
		{name: "policy", service: validService, mutate: func(request *CheckRequest) { request.Policy = core.EffectivePolicy{} }, want: "policy is required"},
		{name: "evaluation time", service: validService, mutate: func(request *CheckRequest) { request.EvaluatedAt = time.Time{} }, want: "evaluation time is required"},
		{name: "engine version", service: validService, mutate: func(request *CheckRequest) { request.EngineVersion = "" }, want: "engine version is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			if tt.mutate != nil {
				tt.mutate(&request)
			}
			_, err := tt.service.Run(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckServiceWrapsInputRoleErrors(t *testing.T) {
	policy, err := core.MergePolicy(core.PolicyLayer{
		Name:                   "stable",
		Source:                 core.PolicySourceRepository,
		RequireReleaseBaseline: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	release := core.ReviewInputLocator{File: "release.yaml"}
	request := CheckRequest{
		ContractID:    "payments",
		SpecPath:      "docs/openapi.yaml",
		Target:        core.ReviewInputLocator{File: "target.yaml"},
		Candidate:     core.ReviewInputLocator{File: "candidate.yaml"},
		Release:       &release,
		Policy:        policy,
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		EngineVersion: "test-engine",
	}

	for _, role := range []string{"target", "candidate", "release"} {
		t.Run(role, func(t *testing.T) {
			loader := &checkInputLoaderFake{failLocator: map[string]error{
				role + ".yaml": errors.New("unavailable"),
			}}
			_, err := (CheckService{Inputs: loader, Snapshots: &checkSnapshotBuilderFake{}}).Run(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), "load "+role+": unavailable") {
				t.Fatalf("Run error = %v", err)
			}
		})
	}
}

type checkInputLoaderFake struct {
	order       *[]string
	specPaths   []string
	locators    []core.ReviewInputLocator
	failLocator map[string]error
}

func (f *checkInputLoaderFake) Load(_ context.Context, specPath string, locator core.ReviewInputLocator) (core.SpecFile, core.Revision, error) {
	label := locator.File
	if label == "" {
		label = locator.GitRef
	}
	if f.order != nil {
		*f.order = append(*f.order, "load:"+label)
	}
	f.specPaths = append(f.specPaths, specPath)
	f.locators = append(f.locators, locator)
	if err := f.failLocator[label]; err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	return core.SpecFile{Path: specPath, Bytes: []byte(label)}, core.Revision{ID: strings.TrimSuffix(label, ".yaml") + "-revision"}, nil
}

type checkSnapshotBuilderFake struct {
	order       *[]string
	contractIDs []string
}

func (f *checkSnapshotBuilderFake) Build(_ context.Context, contractID string, file core.SpecFile, revision core.Revision) (core.ContractSnapshot, error) {
	if f.order != nil {
		*f.order = append(*f.order, "build:"+revision.ID)
	}
	f.contractIDs = append(f.contractIDs, contractID)
	return core.NewContractSnapshot(contractID, revision.ID, file.Bytes, core.SpecIndex{}), nil
}

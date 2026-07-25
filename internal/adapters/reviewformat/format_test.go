package reviewformat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/araihu/manja/internal/core"
)

func TestWriteJSONUsesCanonicalReviewJSON(t *testing.T) {
	report := core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    "payments",
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest:  "policy-digest",
		Verdict:       core.VerdictPass,
		Comparisons: []core.ComparisonReport{{
			Kind:      core.ComparisonPullRequest,
			Baseline:  core.SnapshotRef{RevisionID: "target"},
			Candidate: core.SnapshotRef{RevisionID: "head"},
			Policy:    core.PolicyResult{Verdict: core.VerdictPass},
		}},
	}
	var got bytes.Buffer
	if err := Write(&got, FormatJSON, report); err != nil {
		t.Fatal(err)
	}
	want, err := core.CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("json differs:\n%s\nwant:\n%s", got.Bytes(), want)
	}
}

func TestWriteTextAndMarkdownMatchGoldenFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		format string
		report core.ReviewReport
		golden string
	}{
		{
			name:   "passing text report",
			format: FormatText,
			report: passReport(),
			golden: "pass.txt.golden",
		},
		{
			name:   "failing text report",
			format: FormatText,
			report: failReport(),
			golden: "fail.txt.golden",
		},
		{
			name:   "failing markdown report",
			format: FormatMarkdown,
			report: failReport(),
			golden: "fail.md.golden",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := Write(&got, test.format, test.report); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", test.golden))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatalf("output differs:\n%s\nwant:\n%s", got.Bytes(), want)
			}
			if bytes.Count(got.Bytes(), []byte("\n")) == 0 || got.Bytes()[len(got.Bytes())-1] != '\n' || bytes.HasSuffix(got.Bytes(), []byte("\n\n")) {
				t.Fatalf("output must end with exactly one newline: %q", got.Bytes())
			}
		})
	}
}

func TestWriteRejectsUnknownFormat(t *testing.T) {
	var got bytes.Buffer
	if err := Write(&got, "yaml", passReport()); err == nil {
		t.Fatal("Write() error = nil")
	}
}

func passReport() core.ReviewReport {
	return core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    "payments",
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest:  "policy-digest",
		Verdict:       core.VerdictPass,
		Comparisons: []core.ComparisonReport{{
			Kind:      core.ComparisonPullRequest,
			Baseline:  core.SnapshotRef{RevisionID: "target"},
			Candidate: core.SnapshotRef{RevisionID: "head"},
			Policy:    core.PolicyResult{Verdict: core.VerdictPass},
		}},
	}
}

func failReport() core.ReviewReport {
	finding := core.SpecChange{
		ID:          "finding-1",
		RuleID:      core.RuleOperationRemoved,
		Severity:    core.SpecChangeBreaking,
		Kind:        "Removed endpoint",
		Subject:     "GET /payments",
		Description: "Endpoint exists in production docs but is missing from the candidate.",
	}
	exception := core.PolicyException{
		FindingID: "finding-1",
		Reason:    "v2 migration",
		Author:    "api-team",
		ExpiresAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
		Source:    core.PolicySourceRepository,
	}
	return core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    "payments",
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest:  "policy-digest",
		Verdict:       core.VerdictFail,
		Comparisons: []core.ComparisonReport{
			{
				Kind:      core.ComparisonPullRequest,
				Baseline:  core.SnapshotRef{RevisionID: "target"},
				Candidate: core.SnapshotRef{RevisionID: "head"},
				Findings:  []core.SpecChange{finding},
				Policy: core.PolicyResult{
					Verdict:           core.VerdictFail,
					Decisions:         []core.FindingDecision{{Finding: finding, Level: core.RuleLevelFail, Source: core.PolicySourceRepository}},
					AppliedExceptions: []core.PolicyException{exception},
				},
			},
			{
				Kind:      core.ComparisonReleaseImpact,
				Baseline:  core.SnapshotRef{RevisionID: "release"},
				Candidate: core.SnapshotRef{RevisionID: "head"},
				Findings:  []core.SpecChange{finding},
				Policy: core.PolicyResult{
					Verdict:   core.VerdictFail,
					Decisions: []core.FindingDecision{{Finding: finding, Level: core.RuleLevelFail, Source: core.PolicySourceRepository}},
				},
			},
		},
	}
}

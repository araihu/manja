// Package reviewformat writes deterministic human and machine review reports.
package reviewformat

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/araihu/manja/internal/core"
)

const (
	FormatJSON     = "json"
	FormatText     = "text"
	FormatMarkdown = "markdown"
)

// Write emits report in the requested deterministic format.
func Write(w io.Writer, format string, report core.ReviewReport) error {
	switch format {
	case FormatJSON:
		encoded, err := core.CanonicalReviewJSON(report)
		if err != nil {
			return fmt.Errorf("canonical review JSON: %w", err)
		}
		return writeBytes(w, append(encoded, '\n'))
	case FormatText:
		return writeBytes(w, []byte(formatText(report)))
	case FormatMarkdown:
		return writeBytes(w, []byte(formatMarkdown(report)))
	default:
		return fmt.Errorf("unknown review format %q", format)
	}
}

func formatText(report core.ReviewReport) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Contract: %s\n", visibleText(report.ContractID))
	fmt.Fprintf(&output, "Engine: %s\n", visibleText(report.EngineVersion))
	fmt.Fprintf(&output, "Evaluated at: %s\n", formatTime(report.EvaluatedAt))
	fmt.Fprintf(&output, "Verdict: %s\n", visibleText(report.Verdict))

	for _, comparison := range report.Comparisons {
		output.WriteByte('\n')
		fmt.Fprintf(
			&output,
			"%s (%s -> %s): %s\n",
			visibleText(comparisonTitle(comparison.Kind)),
			visibleText(comparison.Baseline.RevisionID),
			visibleText(comparison.Candidate.RevisionID),
			visibleText(comparison.Policy.Verdict),
		)
		writeTextFindings(&output, comparison.Policy.Decisions)
		writeTextExceptions(&output, comparison.Policy.AppliedExceptions)
	}

	output.WriteByte('\n')
	fmt.Fprintf(&output, "Effective policy digest: %s\n", visibleText(report.PolicyDigest))
	return output.String()
}

func writeTextFindings(output *strings.Builder, decisions []core.FindingDecision) {
	if len(decisions) == 0 {
		output.WriteString("  Findings: none\n")
		return
	}
	output.WriteString("  Findings:\n")
	for _, decision := range decisions {
		fmt.Fprintf(
			output,
			"  - %s %s %s: %s\n",
			visibleText(string(decision.Level)),
			visibleText(decision.Finding.RuleID),
			visibleText(decision.Finding.Subject),
			visibleText(decision.Finding.Description),
		)
	}
}

func writeTextExceptions(output *strings.Builder, exceptions []core.PolicyException) {
	if len(exceptions) == 0 {
		output.WriteString("  Applied exceptions: none\n")
		return
	}
	output.WriteString("  Applied exceptions:\n")
	for _, exception := range exceptions {
		fmt.Fprintf(
			output,
			"  - reason: %s; author: %s; expires: %s\n",
			visibleText(exception.Reason),
			visibleText(exception.Author),
			formatTime(exception.ExpiresAt),
		)
	}
}

func formatMarkdown(report core.ReviewReport) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# Contract review: %s\n\n", markdownText(report.ContractID))
	fmt.Fprintf(&output, "- Engine: %s\n", markdownText(report.EngineVersion))
	fmt.Fprintf(&output, "- Evaluated at: %s\n", formatTime(report.EvaluatedAt))
	fmt.Fprintf(&output, "- Verdict: %s\n", markdownText(report.Verdict))

	for _, comparison := range report.Comparisons {
		fmt.Fprintf(&output, "\n## %s\n\n", markdownText(comparisonTitle(comparison.Kind)))
		fmt.Fprintf(&output, "- Baseline: %s\n", markdownText(comparison.Baseline.RevisionID))
		fmt.Fprintf(&output, "- Candidate: %s\n", markdownText(comparison.Candidate.RevisionID))
		fmt.Fprintf(&output, "- Verdict: %s\n", markdownText(comparison.Policy.Verdict))
		writeMarkdownFindings(&output, comparison.Policy.Decisions)
		writeMarkdownExceptions(&output, comparison.Policy.AppliedExceptions)
	}

	fmt.Fprintf(&output, "\n## Effective policy\n\n- Digest: %s\n", markdownText(report.PolicyDigest))
	return output.String()
}

func writeMarkdownFindings(output *strings.Builder, decisions []core.FindingDecision) {
	if len(decisions) == 0 {
		output.WriteString("- Findings: none\n")
		return
	}
	output.WriteString("- Findings:\n")
	for _, decision := range decisions {
		fmt.Fprintf(
			output,
			"  - %s %s %s: %s\n",
			markdownCode(string(decision.Level)),
			markdownCode(decision.Finding.RuleID),
			markdownCode(decision.Finding.Subject),
			markdownText(decision.Finding.Description),
		)
	}
}

func writeMarkdownExceptions(output *strings.Builder, exceptions []core.PolicyException) {
	if len(exceptions) == 0 {
		output.WriteString("- Applied exceptions: none\n")
		return
	}
	output.WriteString("- Applied exceptions:\n")
	for _, exception := range exceptions {
		fmt.Fprintf(output, "  - Reason: %s\n", markdownText(exception.Reason))
		fmt.Fprintf(output, "    Author: %s\n", markdownText(exception.Author))
		fmt.Fprintf(output, "    Expires: %s\n", formatTime(exception.ExpiresAt))
	}
}

func comparisonTitle(kind string) string {
	switch kind {
	case core.ComparisonPullRequest:
		return "Pull-request delta"
	case core.ComparisonReleaseImpact:
		return "Release impact"
	default:
		return kind
	}
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func visibleText(value string) string {
	var output strings.Builder
	for _, char := range value {
		switch char {
		case '\n':
			output.WriteString(`\n`)
		case '\r':
			output.WriteString(`\r`)
		case '\t':
			output.WriteString(`\t`)
		default:
			switch {
			case unicode.Is(unicode.Cc, char) || unicode.Is(unicode.Cf, char) ||
				unicode.Is(unicode.Zl, char) || unicode.Is(unicode.Zp, char):
				if char <= '\uffff' {
					fmt.Fprintf(&output, `\u%04x`, char)
				} else {
					fmt.Fprintf(&output, `\U%08x`, char)
				}
			default:
				output.WriteRune(char)
			}
		}
	}
	return output.String()
}

func markdownText(value string) string {
	value = visibleText(value)
	var output strings.Builder
	for _, char := range value {
		if strings.ContainsRune(`\`+"`*_{}[]()#!|<>&", char) {
			output.WriteByte('\\')
		}
		output.WriteRune(char)
	}
	return output.String()
}

func markdownCode(value string) string {
	value = visibleText(value)
	longestRun := 0
	currentRun := 0
	for _, char := range value {
		if char == '`' {
			currentRun++
			if currentRun > longestRun {
				longestRun = currentRun
			}
			continue
		}
		currentRun = 0
	}
	fence := strings.Repeat("`", longestRun+1)
	padding := ""
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") ||
		strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		padding = " "
	}
	return fence + padding + value + padding + fence
}

func writeBytes(w io.Writer, bytes []byte) error {
	written, err := w.Write(bytes)
	if err != nil {
		return err
	}
	if written != len(bytes) {
		return io.ErrShortWrite
	}
	return nil
}

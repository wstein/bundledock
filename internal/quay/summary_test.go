package quay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanSummaryListsCountsAndRows(t *testing.T) {
	plan := Plan{Schema: 1, Mode: ModePublication, Rows: []PlanRow{
		{Repository: "toolbox", Name: "staging-1-amd64", Eligible: true, Reason: "Verified publication staging tag"},
		{Repository: "toolbox", Name: "staging-2-amd64", Reason: "pipe | and\nnewline"},
	}}
	got := PlanSummary(plan)
	for _, want := range []string{"### Staging cleanup (publication)", "Staging tags: 2; eligible: 1 (dry run, nothing changed)", "| toolbox | `staging-1-amd64` | eligible | Verified publication staging tag |", "| toolbox | `staging-2-amd64` | retained | pipe \\| and newline |"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") || !strings.HasPrefix(got, "\n") {
		t.Errorf("section must be separated by blank lines: %q", got)
	}
}

func TestPlanSummaryTruncatesLongPlans(t *testing.T) {
	plan := Plan{Schema: 1, Mode: ModeFallback}
	for i := 0; i < 250; i++ {
		plan.Rows = append(plan.Rows, PlanRow{Repository: "toolbox", Name: fmt.Sprintf("staging-%d-amd64", i), Reason: "x"})
	}
	got := PlanSummary(plan)
	if !strings.Contains(got, "Staging tags: 250;") || !strings.Contains(got, "150 more rows are in the saved plan") || strings.Contains(got, "staging-249-amd64") {
		t.Fatalf("summary = %.400s", got)
	}
}

func TestApplySummaryShowsActionsAndASafeStopReason(t *testing.T) {
	actions := []Action{{Repository: "toolbox", Name: "staging-1-amd64", Action: "deleted"}, {Repository: "toolbox", Name: "staging-1-arm64", Action: "already absent"}}
	got := ApplySummary(ModeFailedRun, &FailedRun{RunID: "123", Attempt: 2}, actions, nil)
	for _, want := range []string{"### Staging cleanup apply (failed-run)", "Run 123 attempt 2", "Actions: 2; deleted: 1", "| toolbox | `staging-1-amd64` | deleted |", "already absent"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "Stopped") {
		t.Error("a complete run reports a stop")
	}
	for err, want := range map[error]string{
		reasonError("Staging tag no longer eligible"): "Stopped: Staging tag no longer eligible",
		&StatusError{Code: 500}:                       "Stopped: Quay HTTP 500",
		fmt.Errorf("open /secret/path: denied"):       "Stopped: see the job log",
	} {
		got := ApplySummary(ModeFallback, nil, actions, err)
		if !strings.Contains(got, want) || strings.Contains(got, "secret") {
			t.Errorf("%v: %s", err, got)
		}
	}
}

func TestAppendStepSummaryIsAppendOnlyAndOptional(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	os.WriteFile(path, []byte("earlier step\n"), 0o644)
	if err := AppendStepSummary(path, "\nfirst\n"); err != nil {
		t.Fatal(err)
	}
	if err := AppendStepSummary(path, "\nsecond\n"); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "earlier step\n\nfirst\n\nsecond\n" {
		t.Fatalf("summary = %q", raw)
	}
	// Without the variable nothing is written and nothing fails.
	if err := AppendStepSummary("", "x"); err != nil {
		t.Fatal(err)
	}
	// A missing parent directory is an error, never a created file.
	if err := AppendStepSummary(filepath.Join(t.TempDir(), "none", "summary.md"), "x"); err == nil {
		t.Fatal("unwritable summary reported success")
	}
}

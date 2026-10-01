package github_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

func fakeRunner(outputs map[string]string, calls *[][]string) github.Runner {
	var mu sync.Mutex
	return func(_ context.Context, args ...string) ([]byte, error) {
		mu.Lock()
		*calls = append(*calls, args)
		mu.Unlock()
		out, ok := outputs[args[2]]
		if !ok {
			return nil, errors.New("exit status 1")
		}
		return []byte(out), nil
	}
}

func TestPRStatusParsesGhOutput(t *testing.T) {
	var calls [][]string
	run := fakeRunner(map[string]string{
		"https://github.com/o/r/pull/7": `{"number":7,"state":"OPEN","isDraft":true,"reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-09-30T10:00:00Z"}]}`,
	}, &calls)

	st, err := github.NewClient(run).PRStatus(context.Background(), "https://github.com/o/r/pull/7")

	if err != nil {
		t.Fatal(err)
	}
	want := github.PRStatus{Number: 7, State: "OPEN", IsDraft: true, Review: "REVIEW_REQUIRED", CI: github.CIPassing}
	if st != want {
		t.Fatalf("got %+v want %+v", st, want)
	}
	wantArgs := []string{"pr", "view", "https://github.com/o/r/pull/7", "--json", "number,state,isDraft,reviewDecision,statusCheckRollup"}
	if !reflect.DeepEqual(calls[0], wantArgs) {
		t.Fatalf("args %v", calls[0])
	}
}

func TestPRStatusRunnerError(t *testing.T) {
	var calls [][]string
	_, err := github.NewClient(fakeRunner(nil, &calls)).PRStatus(context.Background(), "https://github.com/o/r/pull/1")

	if err == nil {
		t.Fatal("want error")
	}
}

func TestCIRollup(t *testing.T) {
	cases := []struct {
		name   string
		rollup string
		want   github.CI
	}{
		{"no checks", `[]`, github.CINone},
		{"all passing, skipped and neutral count as passing", `[
			{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SUCCESS"},
			{"__typename":"CheckRun","name":"b","status":"COMPLETED","conclusion":"SKIPPED"},
			{"__typename":"CheckRun","name":"c","status":"COMPLETED","conclusion":"NEUTRAL"}]`, github.CIPassing},
		{"in progress check", `[
			{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SUCCESS"},
			{"__typename":"CheckRun","name":"b","status":"IN_PROGRESS","conclusion":""}]`, github.CIPending},
		{"failure beats pending", `[
			{"__typename":"CheckRun","name":"a","status":"IN_PROGRESS"},
			{"__typename":"CheckRun","name":"b","status":"COMPLETED","conclusion":"FAILURE"}]`, github.CIFailing},
		{"cancelled is not a failure", `[
			{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"CANCELLED"}]`, github.CIPassing},
		{"status context failure", `[{"__typename":"StatusContext","context":"ci/semaphore","state":"FAILURE"}]`, github.CIFailing},
		{"status context pending", `[{"__typename":"StatusContext","context":"ci/semaphore","state":"PENDING"}]`, github.CIPending},
		{"rerun supersedes earlier failure", `[
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-09-30T10:00:00Z"},
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-09-30T11:00:00Z"}]`, github.CIPassing},
		{"queued rerun with zero start time supersedes earlier failure", `[
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-09-30T10:00:00Z"},
			{"__typename":"CheckRun","workflowName":"CI","name":"test","status":"QUEUED","conclusion":"","startedAt":"0001-01-01T00:00:00Z"}]`, github.CIPending},
		{"newer status context supersedes older", `[
			{"__typename":"StatusContext","context":"ci/semaphore","state":"FAILURE","startedAt":"2026-09-30T10:00:00Z"},
			{"__typename":"StatusContext","context":"ci/semaphore","state":"SUCCESS","startedAt":"2026-09-30T11:00:00Z"}]`, github.CIPassing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			run := fakeRunner(map[string]string{"u": `{"number":1,"state":"OPEN","statusCheckRollup":` + tc.rollup + `}`}, &calls)

			st, err := github.NewClient(run).PRStatus(context.Background(), "u")

			if err != nil {
				t.Fatal(err)
			}
			if st.CI != tc.want {
				t.Fatalf("CI = %v want %v", st.CI, tc.want)
			}
		})
	}
}

func TestStatusesSkipsFailures(t *testing.T) {
	var calls [][]string
	run := fakeRunner(map[string]string{
		"https://github.com/o/r/pull/1": `{"number":1,"state":"OPEN","statusCheckRollup":[]}`,
	}, &calls)

	got := github.NewClient(run).Statuses(context.Background(), []string{"https://github.com/o/r/pull/1", "https://github.com/o/r/pull/2"}, 4)

	if len(got) != 1 || got["https://github.com/o/r/pull/1"].Number != 1 {
		t.Fatalf("got %+v", got)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
}

func TestStatusesWithNonPositiveParallelismStillCompletes(t *testing.T) {
	var calls [][]string
	run := fakeRunner(map[string]string{"u": `{"number":1,"state":"OPEN","statusCheckRollup":[]}`}, &calls)

	got := github.NewClient(run).Statuses(context.Background(), []string{"u"}, 0)

	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

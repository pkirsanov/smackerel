package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// recordingExec captures Exec calls for assertion. It is the minimal
// implementation of the Execer interface used by MergeUserContext.
type recordingExec struct {
	calls []recordedCall
	err   error
}

type recordedCall struct {
	sql  string
	args []any
}

func (r *recordingExec) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	r.calls = append(r.calls, recordedCall{sql: sql, args: args})
	return pgconn.CommandTag{}, r.err
}

// TestMergeUserContext_AppendsContextToMetadata is the adversarial regression
// test for BUG-001. It fails if MergeUserContext does not exist or stops issuing
// the JSONB array-append UPDATE against artifacts.metadata->'user_contexts'.
func TestMergeUserContext_AppendsContextToMetadata(t *testing.T) {
	exec := &recordingExec{}
	err := MergeUserContext(context.Background(), exec, "artifact-abc", "for the team meeting")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("expected exactly one Exec call, got %d", len(exec.calls))
	}
	call := exec.calls[0]
	if !strings.Contains(call.sql, "UPDATE artifacts") {
		t.Errorf("SQL must update artifacts table; got: %s", call.sql)
	}
	if !strings.Contains(call.sql, "metadata") {
		t.Errorf("SQL must operate on metadata column; got: %s", call.sql)
	}
	if !strings.Contains(call.sql, "user_contexts") {
		t.Errorf("SQL must target user_contexts JSONB key; got: %s", call.sql)
	}
	if !strings.Contains(call.sql, "jsonb_build_array") {
		t.Errorf("SQL must wrap the new context in a JSONB array element to support concat; got: %s", call.sql)
	}
	if len(call.args) != 2 {
		t.Fatalf("expected 2 args (newContext, artifactID), got %d", len(call.args))
	}
	if call.args[0] != "for the team meeting" {
		t.Errorf("first bound arg must be the new context string; got %v", call.args[0])
	}
	if call.args[1] != "artifact-abc" {
		t.Errorf("second bound arg must be the artifact ID; got %v", call.args[1])
	}
}

func TestMergeUserContext_NoOpOnEmptyContext(t *testing.T) {
	exec := &recordingExec{}
	if err := MergeUserContext(context.Background(), exec, "artifact-abc", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected zero Exec calls for empty context, got %d", len(exec.calls))
	}
}

func TestMergeUserContext_NoOpOnEmptyArtifactID(t *testing.T) {
	exec := &recordingExec{}
	if err := MergeUserContext(context.Background(), exec, "", "some context"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected zero Exec calls for empty artifact ID, got %d", len(exec.calls))
	}
}

func TestMergeUserContext_PropagatesExecError(t *testing.T) {
	sentinel := errors.New("boom")
	exec := &recordingExec{err: sentinel}
	err := MergeUserContext(context.Background(), exec, "artifact-abc", "ctx")
	if err == nil {
		t.Fatal("expected error to propagate from executor, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped error to match sentinel; got %v", err)
	}
}

func TestMergeUserContext_NilExecutorReturnsError(t *testing.T) {
	err := MergeUserContext(context.Background(), nil, "artifact-abc", "ctx")
	if err == nil {
		t.Fatal("expected error when executor is nil, got nil")
	}
}

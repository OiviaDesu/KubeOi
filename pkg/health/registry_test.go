package health

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

type fakeChecker struct {
	name   string
	result *CheckResult
	err    error
}

func (f fakeChecker) Name() string {
	return f.name
}

func (f fakeChecker) Check(context.Context, *corev1.Node) (*CheckResult, error) {
	return f.result, f.err
}

func TestRegistryCheckNodeNormalizesNilResult(t *testing.T) {
	t.Parallel()

	provider := NewRegistry(logr.Discard())
	if err := provider.RegisterChecker(fakeChecker{name: "nil-checker"}); err != nil {
		t.Fatalf("register checker: %v", err)
	}

	results, err := provider.CheckNode(context.Background(), &corev1.Node{})
	if err != nil {
		t.Fatalf("check node: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0] == nil {
		t.Fatalf("expected normalized non-nil result")
	}
	if results[0].Status != HealthStatusUnknown {
		t.Fatalf("expected unknown status, got %s", results[0].Status)
	}
	if got := results[0].Details["checker"]; got != "nil-checker" {
		t.Fatalf("expected checker detail to be set, got %#v", got)
	}
}

func TestRegistryCheckNodeCapturesCheckerErrorWithMetadata(t *testing.T) {
	t.Parallel()

	provider := NewRegistry(logr.Discard())
	if err := provider.RegisterChecker(fakeChecker{name: "boom", err: errors.New("boom")}); err != nil {
		t.Fatalf("register checker: %v", err)
	}

	results, err := provider.CheckNode(context.Background(), &corev1.Node{})
	if err != nil {
		t.Fatalf("check node: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if got := results[0].Details["checker"]; got != "boom" {
		t.Fatalf("expected checker name in error result, got %#v", got)
	}
	if provider.GetOverallStatus(results) != HealthStatusUnknown {
		t.Fatalf("expected overall status to be unknown for checker error")
	}
}
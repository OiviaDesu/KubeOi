package controllers

import (
	"testing"
	"time"

	"github.com/oiviadesu/oiviak3s-operator/pkg/health"
)

func TestCheckResultCheckerNameFallsBackToUnknown(t *testing.T) {
	t.Parallel()

	if got := checkResultCheckerName(nil); got != "unknown" {
		t.Fatalf("expected unknown for nil result, got %q", got)
	}

	result := &health.CheckResult{Timestamp: time.Now()}
	if got := checkResultCheckerName(result); got != "unknown" {
		t.Fatalf("expected unknown without checker detail, got %q", got)
	}

	result.Details = map[string]interface{}{"checker": "kubelet"}
	if got := checkResultCheckerName(result); got != "kubelet" {
		t.Fatalf("expected kubelet checker name, got %q", got)
	}
}

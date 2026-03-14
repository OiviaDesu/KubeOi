package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/oiviadesu/oiviak3s-operator/pkg/notification"
)

func TestSeverityLevelOrdersHigherSeverityCorrectly(t *testing.T) {
	t.Parallel()

	if severityLevel(notification.SeverityCritical) <= severityLevel(notification.SeverityWarning) {
		t.Fatalf("expected critical severity to rank above warning")
	}

	if severityLevel(notification.SeverityWarning) <= severityLevel(notification.SeverityInfo) {
		t.Fatalf("expected warning severity to rank above info")
	}
}

func TestWaitForDrainTimeoutHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := waitForDrainTimeout(ctx, time.Second); err == nil {
		t.Fatalf("expected context cancellation error")
	}
}
package controllers

import "testing"

func TestDesiredReplicasDefaultsToOne(t *testing.T) {
	t.Parallel()

	if got := desiredReplicas(nil); got != 1 {
		t.Fatalf("expected default desired replicas to be 1, got %d", got)
	}

	two := int32(2)
	if got := desiredReplicas(&two); got != 2 {
		t.Fatalf("expected explicit desired replicas to be preserved, got %d", got)
	}
}
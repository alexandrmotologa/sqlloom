package telemetry

import (
	"testing"
)

func TestInvalidationTrackerAndChurn(t *testing.T) {
	tracker := NewInvalidationTracker()

	// Record normal invalidations
	tracker.RecordInvalidation("users", "UPDATE", 3, 0.25)
	tracker.RecordInvalidation("products", "INSERT", 1, 0.12)

	history := tracker.GetHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(history))
	}
	if history[0].Table != "products" || history[1].Table != "users" {
		t.Fatalf("expected reverse chronological order, got %v", history)
	}

	// Trigger high churn on table 'heartbeats' (more than 6 writes)
	for i := 0; i < 10; i++ {
		tracker.RecordInvalidation("heartbeats", "UPDATE", 0, 0.05)
	}

	recs := tracker.GetRecommendations()
	if len(recs) == 0 {
		t.Fatalf("expected churn recommendation for heartbeats, got none")
	}

	found := false
	for _, r := range recs {
		if r.Table == "heartbeats" {
			found = true
			if r.Severity != "MEDIUM" && r.Severity != "HIGH" {
				t.Fatalf("expected severity MEDIUM or HIGH, got %s", r.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("expected recommendation for heartbeats")
	}
}

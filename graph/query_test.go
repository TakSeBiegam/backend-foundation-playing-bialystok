package graph

import (
    "context"
    "testing"
)

func TestEvents_ReturnsSeededEvents(t *testing.T) {
    ctx := context.Background()
    r := newTestResolver()
    events, err := r.Query().Events(ctx)
    if err != nil {
        t.Fatalf("Events error: %v", err)
    }
    if len(events) == 0 {
        t.Fatalf("expected at least one event")
    }
}

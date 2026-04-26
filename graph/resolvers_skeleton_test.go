package graph

import (
    "context"
    "testing"
)

func ctxWithCaller(role, id, email, name string) context.Context {
    ctx := context.Background()
    ctx = context.WithValue(ctx, "callerRole", role)
    ctx = context.WithValue(ctx, "callerId", id)
    ctx = context.WithValue(ctx, "callerEmail", email)
    ctx = context.WithValue(ctx, "callerName", name)
    return ctx
}

func TestQueries_Skeleton(t *testing.T) {
    r := newTestResolver()

    t.Run("Events_basic", func(t *testing.T) {
        ctx := context.Background()
        events, err := r.Query().Events(ctx)
        if err != nil {
            t.Fatalf("Events error: %v", err)
        }
        if len(events) == 0 {
            t.Fatalf("expected seeded events")
        }
    })

    t.Run("Event_byID", func(t *testing.T) {
        ctx := context.Background()
        events, err := r.Query().Events(ctx)
        if err != nil || len(events) == 0 {
            t.Skip("no events available")
        }
        e, err := r.Query().Event(ctx, events[0].ID)
        if err != nil {
            t.Fatalf("Event() returned error: %v", err)
        }
        if e == nil || e.ID != events[0].ID {
            t.Fatalf("unexpected event")
        }
    })

    t.Run("Partners_basic", func(t *testing.T) {
        ctx := context.Background()
        partners, err := r.Query().Partners(ctx)
        if err != nil {
            t.Fatalf("Partners error: %v", err)
        }
        t.Logf("partners count: %d", len(partners))
    })

    t.Run("BoardGames_basic", func(t *testing.T) {
        ctx := context.Background()
        bgs, err := r.Query().BoardGames(ctx)
        if err != nil {
            t.Fatalf("BoardGames error: %v", err)
        }
        if len(bgs) == 0 {
            t.Fatalf("expected seeded board games")
        }
    })

    t.Run("Queries_others_skip", func(t *testing.T) {
        t.Skip("skeleton: implement tests for remaining queries")
    })
}

func TestMutations_Skeleton(t *testing.T) {
    t.Run("Mutations_others_skip", func(t *testing.T) {
        t.Skip("skeleton: implement mutation tests for other resolvers")
    })
}

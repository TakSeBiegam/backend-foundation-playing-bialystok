package graph

import (
    "context"
    "log"
    "os"
    "path/filepath"
    "testing"
    "time"

    "backend/bootstrap"
    "github.com/jackc/pgx/v5/pgxpool"
)

var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
    dbURL := os.Getenv("TEST_DATABASE_URL")
    if dbURL == "" {
        dbURL = "postgres://gbialystok:gbialystok_secret@localhost:5433/gbialystok?sslmode=disable"
    }
    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()
    pool, err := pgxpool.New(ctx, dbURL)
    if err != nil {
        log.Fatalf("connect to test db: %v", err)
    }
    testDB = pool

    // Ensure schema + seeds via bootstrap (idempotent)
    if err := bootstrap.EnsureBoardGamesSchema(ctx, pool); err != nil {
        log.Printf("warning: EnsureBoardGamesSchema: %v", err)
    }
    if err := bootstrap.EnsureBoardGamesSeed(ctx, pool); err != nil {
        log.Printf("warning: EnsureBoardGamesSeed: %v", err)
    }
    if err := bootstrap.EnsureAboutUsOfferBlocksSeed(ctx, pool); err != nil {
        log.Printf("warning: EnsureAboutUsOfferBlocksSeed: %v", err)
    }

    // Try applying init.sql as fallback (idempotent)
    initPath := filepath.Join("..", "..", "db", "init.sql")
    b, err := os.ReadFile(initPath)
    if err != nil {
        log.Printf("warning: could not read %s: %v", initPath, err)
    } else {
        if _, err := testDB.Exec(ctx, string(b)); err != nil {
            log.Printf("warning: executing init.sql: %v", err)
        }
    }

    code := m.Run()
    pool.Close()
    os.Exit(code)
}

func truncateAllTables(t *testing.T, ctx context.Context) {
    t.Helper()
    if testDB == nil {
        t.Fatal("testDB is nil")
    }
    _, err := testDB.Exec(ctx, `
        TRUNCATE TABLE audit_logs, contact_submission_notes, contact_submissions, offer_blocks, board_games, partners, events, role_permissions, users RESTART IDENTITY CASCADE;
    `)
    if err != nil {
        t.Fatalf("truncate failed: %v", err)
    }
}

func newTestResolver() *Resolver {
    return &Resolver{DB: testDB}
}

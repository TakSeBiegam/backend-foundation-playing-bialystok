package graph

import (
    "context"
    "testing"

    "backend/graph/model"
)

func TestCreateContactSubmission_InsertsAndReturns(t *testing.T) {
    ctx := context.Background()
    truncateAllTables(t, ctx)
    r := newTestResolver()
    phone := "123456"
    input := model.CreateContactSubmissionInput{
        FirstName: "Test",
        LastName:  "User",
        Phone:     &phone,
        Message:   "Hello from test",
    }
    got, err := r.Mutation().CreateContactSubmission(ctx, input)
    if err != nil {
        t.Fatalf("CreateContactSubmission error: %v", err)
    }
    if got == nil {
        t.Fatalf("expected non-nil result")
    }
    if got.Message != input.Message || got.FirstName != input.FirstName || got.LastName != input.LastName {
        t.Fatalf("unexpected fields: %+v", got)
    }
    if got.Phone == nil || input.Phone == nil || *got.Phone != *input.Phone {
        t.Fatalf("unexpected fields: %+v", got)
    }
}

func TestUpdateBoardGame_ClearsImageFields(t *testing.T) {
    r := newTestResolver()

    t.Run("clears image and alt together", func(t *testing.T) {
        ctx := context.Background()
        truncateAllTables(t, ctx)

        var actorID string
        err := testDB.QueryRow(ctx,
            `INSERT INTO users (email, username, password_hash, role)
             VALUES ($1, $2, $3, $4)
             RETURNING id`,
            "owner@example.com",
            "owner",
            "test-hash",
            "owner",
        ).Scan(&actorID)
        if err != nil {
            t.Fatalf("insert owner: %v", err)
        }

        var gameID string
        err = testDB.QueryRow(ctx,
            `INSERT INTO board_games (title, description, player_bucket, image_url, image_alt)
             VALUES ($1, $2, $3, $4, $5)
             RETURNING id`,
            "Azul",
            "Opis testowy planszowki",
            "2-4",
            "/uploads/azul.webp",
            "Pudelko gry Azul",
        ).Scan(&gameID)
        if err != nil {
            t.Fatalf("insert board game: %v", err)
        }

        clearImageURL := true
        updatedGame, err := r.Mutation().UpdateBoardGame(
            ctxWithCaller("OWNER", actorID, "owner@example.com", "Owner"),
            gameID,
            model.UpdateBoardGameInput{
                ClearImageURL: &clearImageURL,
            },
        )
        if err != nil {
            t.Fatalf("UpdateBoardGame error: %v", err)
        }
        if updatedGame == nil {
            t.Fatalf("expected updated board game")
        }
        if updatedGame.ImageURL != nil {
            t.Fatalf("expected image url to be nil, got %v", *updatedGame.ImageURL)
        }
        if updatedGame.ImageAlt != nil {
            t.Fatalf("expected image alt to be nil, got %v", *updatedGame.ImageAlt)
        }

        var storedImageURL *string
        var storedImageAlt *string
        err = testDB.QueryRow(ctx,
            `SELECT image_url, image_alt FROM board_games WHERE id = $1`,
            gameID,
        ).Scan(&storedImageURL, &storedImageAlt)
        if err != nil {
            t.Fatalf("load stored board game: %v", err)
        }
        if storedImageURL != nil || storedImageAlt != nil {
            t.Fatalf("expected stored image fields to be cleared, got url=%v alt=%v", storedImageURL, storedImageAlt)
        }
    })

    t.Run("clears alt without removing image", func(t *testing.T) {
        ctx := context.Background()
        truncateAllTables(t, ctx)

        var actorID string
        err := testDB.QueryRow(ctx,
            `INSERT INTO users (email, username, password_hash, role)
             VALUES ($1, $2, $3, $4)
             RETURNING id`,
            "owner2@example.com",
            "owner2",
            "test-hash",
            "owner",
        ).Scan(&actorID)
        if err != nil {
            t.Fatalf("insert owner: %v", err)
        }

        var gameID string
        err = testDB.QueryRow(ctx,
            `INSERT INTO board_games (title, description, player_bucket, image_url, image_alt)
             VALUES ($1, $2, $3, $4, $5)
             RETURNING id`,
            "Splendor",
            "Opis testowy planszowki",
            "2-4",
            "/uploads/splendor.webp",
            "Pudelko gry Splendor",
        ).Scan(&gameID)
        if err != nil {
            t.Fatalf("insert board game: %v", err)
        }

        clearImageAlt := true
        updatedGame, err := r.Mutation().UpdateBoardGame(
            ctxWithCaller("OWNER", actorID, "owner2@example.com", "Owner"),
            gameID,
            model.UpdateBoardGameInput{
                ClearImageAlt: &clearImageAlt,
            },
        )
        if err != nil {
            t.Fatalf("UpdateBoardGame error: %v", err)
        }
        if updatedGame == nil {
            t.Fatalf("expected updated board game")
        }
        if updatedGame.ImageURL == nil || *updatedGame.ImageURL != "/uploads/splendor.webp" {
            t.Fatalf("expected image url to stay unchanged, got %v", updatedGame.ImageURL)
        }
        if updatedGame.ImageAlt != nil {
            t.Fatalf("expected image alt to be nil, got %v", *updatedGame.ImageAlt)
        }
    })
}

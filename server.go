package main

import (
	"backend/graph"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"bytes"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vektah/gqlparser/v2/ast"
)

const defaultPort = "8080"

func ensureContactSubmissionSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS contact_submissions (
			id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
			first_name       VARCHAR(120) NOT NULL,
			last_name        VARCHAR(120) NOT NULL,
			phone            VARCHAR(50),
			message          TEXT         NOT NULL,
			is_read          BOOLEAN      NOT NULL DEFAULT FALSE,
			read_at          TIMESTAMPTZ,
			read_by_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
			archived         BOOLEAN      NOT NULL DEFAULT FALSE,
			last_note_at     TIMESTAMPTZ,
			created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS contact_submission_notes (
			id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			submission_id  UUID        NOT NULL REFERENCES contact_submissions(id) ON DELETE CASCADE,
			author_user_id UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
			note           TEXT        NOT NULL,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS contact_submissions_created_at_idx ON contact_submissions(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS contact_submissions_is_read_idx ON contact_submissions(is_read)`,
		`CREATE INDEX IF NOT EXISTS contact_submissions_read_by_user_id_idx ON contact_submissions(read_by_user_id)`,
		`CREATE INDEX IF NOT EXISTS contact_submission_notes_submission_idx ON contact_submission_notes(submission_id)`,
		`CREATE INDEX IF NOT EXISTS contact_submission_notes_created_at_idx ON contact_submission_notes(created_at DESC)`,
		`ALTER TABLE contact_submissions ADD COLUMN IF NOT EXISTS archived BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE contact_submissions ADD COLUMN IF NOT EXISTS last_note_at TIMESTAMPTZ`,
		`CREATE INDEX IF NOT EXISTS contact_submissions_archived_idx ON contact_submissions(archived)`,
		`CREATE INDEX IF NOT EXISTS contact_submissions_last_note_at_idx ON contact_submissions(last_note_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply contact schema statement: %w", err)
		}
	}

	return nil
}

func corsMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://gbialystok:gbialystok_secret@localhost:5432/gbialystok"
	}

	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:3001"
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}
	log.Println("connected to database")

	if err := ensureContactSubmissionSchema(context.Background(), pool); err != nil {
		log.Fatalf("unable to ensure contact schema: %v", err)
	}

	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{DB: pool},
	}))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	cors := corsMiddleware(corsOrigin)

	http.Handle("/", cors(playground.Handler("GraphQL playground", "/query")))
	http.Handle("/query", cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// read body so we can inspect operation and variables for authorization
		var bodyBytes []byte
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			bodyBytes = b
		}

		// restore body for downstream handler
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// determine caller role from proxy headers
		callerRole := strings.ToUpper(strings.TrimSpace(r.Header.Get("X-User-Role")))

		// naive GraphQL operation inspection (sufficient for admin panel patterns)
		var payload map[string]any
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &payload)
		}
		queryStr := ""
		if q, ok := payload["query"].(string); ok {
			queryStr = q
		}

		variables := map[string]any{}
		if v, ok := payload["variables"].(map[string]any); ok {
			variables = v
		}

		// helper to reject with GraphQL error JSON
		reject := func(msg string, code int) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"message": msg}}, "data": nil})
		}

		// Authorization rules:
		// - only ADMIN or OWNER can list/create/delete/reset users
		// - only OWNER can assign OWNER role
		q := strings.ToLower(queryStr)
		if strings.Contains(q, "createuser") || strings.Contains(q, "deleteuser") || strings.Contains(q, "resetuserpassword") || strings.Contains(q, "updateuser") || strings.Contains(q, "users") {
			if callerRole == "" {
				reject("unauthenticated", 401)
				return
			}
			// create/delete/reset/list require ADMIN or OWNER
			if strings.Contains(q, "createuser") || strings.Contains(q, "deleteuser") || strings.Contains(q, "resetuserpassword") || strings.Contains(q, "users") {
				if callerRole != "ADMIN" && callerRole != "OWNER" {
					reject("forbidden: insufficient permissions", 403)
					return
				}
			}

			// updateUser: if role change to OWNER -> only OWNER allowed; if any role change -> ADMIN/OWNER only
			if strings.Contains(q, "updateuser") {
				if input, ok := variables["input"].(map[string]any); ok {
					if roleVal, ok := input["role"].(string); ok {
						if strings.ToUpper(roleVal) == "OWNER" && callerRole != "OWNER" {
							reject("forbidden: only OWNER can assign OWNER role", 403)
							return
						}
						if callerRole != "ADMIN" && callerRole != "OWNER" {
							reject("forbidden: insufficient permissions to change roles", 403)
							return
						}
					}
				}
			}
		}

		// propagate caller info from proxy headers into context for resolvers
		ctx := context.WithValue(r.Context(), "callerRole", r.Header.Get("X-User-Role"))
		ctx = context.WithValue(ctx, "callerId", r.Header.Get("X-User-Id"))
		srv.ServeHTTP(w, r.WithContext(ctx))
	})))

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

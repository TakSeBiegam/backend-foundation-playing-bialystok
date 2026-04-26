package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"backend/graph/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type statuteResponse struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	UpdatedAt string `json:"updatedAt"`
}

type statuteVersionResponse struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Summary   *string `json:"summary"`
	AuthorID  *string `json:"authorId"`
	CreatedAt string  `json:"createdAt"`
}

// StatuteHandler returns an http.Handler that exposes GET (current statute)
// and POST (update statute) endpoints. POST requires X-User-Role header to be
// ADMIN or OWNER. Uses the provided pgxpool for DB access.
func StatuteHandler(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetStatute(w, r, pool)
		case http.MethodPost:
			handlePostStatute(w, r, pool)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func StatuteVersionsHandler(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		limit := 50
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = v
			}
		}

		ctx := r.Context()
		rows, err := pool.Query(ctx, `
            SELECT id, content, summary, author_user_id, created_at
            FROM statute_versions
            ORDER BY created_at DESC
            LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("query statute versions: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var out []statuteVersionResponse
		for rows.Next() {
			var v statuteVersionResponse
			var authorID *string
			var summary *string
			var createdAt time.Time
			if err := rows.Scan(&v.ID, &v.Content, &summary, &authorID, &createdAt); err != nil {
				http.Error(w, fmt.Sprintf("scan statute version: %v", err), http.StatusInternalServerError)
				return
			}
			v.Summary = summary
			v.AuthorID = authorID
			v.CreatedAt = createdAt.Format(time.RFC3339)
			out = append(out, v)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

func handleGetStatute(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	ctx := r.Context()

	var id string
	var content string
	var updatedAt time.Time

	err := pool.QueryRow(ctx, `SELECT id, content, updated_at FROM statutes LIMIT 1`).Scan(&id, &content, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(statuteResponse{ID: "", Content: "", UpdatedAt: ""})
			return
		}

		http.Error(w, fmt.Sprintf("query statute: %v", err), http.StatusInternalServerError)
		return
	}

	resp := statuteResponse{ID: id, Content: content, UpdatedAt: updatedAt.Format(time.RFC3339)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handlePostStatute(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	// Require role: ADMIN or OWNER
	role := r.Header.Get("X-User-Role")
	if role == "" {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if role != "OWNER" && role != "ADMIN" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var payload struct {
		Content string  `json:"content"`
		Summary *string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	tx, err := pool.Begin(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("begin tx: %v", err), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Ensure there is a statutes row
	var statuteID string
	var prevContent string
	err = tx.QueryRow(ctx, `SELECT id, content FROM statutes LIMIT 1`).Scan(&statuteID, &prevContent)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, fmt.Sprintf("query statute: %v", err), http.StatusInternalServerError)
			return
		}

		prevContent = ""
		err = tx.QueryRow(ctx, `INSERT INTO statutes (content) VALUES ('') RETURNING id`).Scan(&statuteID)
		if err != nil {
			http.Error(w, fmt.Sprintf("insert statute: %v", err), http.StatusInternalServerError)
			return
		}
	}

	var updatedAt time.Time

	// Insert version with the saved content so history reflects the actual saved revision.
	authorID := r.Header.Get("X-User-Id")
	_, err = tx.Exec(ctx, `INSERT INTO statute_versions (statute_id, content, summary, author_user_id) VALUES ($1,$2,$3,$4)`, statuteID, payload.Content, payload.Summary, nullString(authorID))
	if err != nil {
		http.Error(w, fmt.Sprintf("insert statute version: %v", err), http.StatusInternalServerError)
		return
	}

	// Update statutes row
	err = tx.QueryRow(ctx, `UPDATE statutes SET content = $1, updated_at = NOW() WHERE id = $2 RETURNING updated_at`, payload.Content, statuteID).Scan(&updatedAt)
	if err != nil {
		http.Error(w, fmt.Sprintf("update statute: %v", err), http.StatusInternalServerError)
		return
	}

	// Audit log
	var actorRole *model.Role
	if parsedRole, parseErr := roleFromDBString(role); parseErr == nil {
		actorRole = &parsedRole
	}
	var actorUserID *string
	if authorID != "" {
		actorUserID = &authorID
	}
	_ = insertAuditLogWithExec(ctx, tx, auditLogEntry{
		ActorUserID: actorUserID,
		ActorRole:   actorRole,
		Resource:    model.AdminResource("STATUTE"),
		Action:      model.AuditActionUpdate,
		ResourceID:  &statuteID,
		Summary:     fmt.Sprintf("Aktualizacja regulaminu: %s", firstN(payload.Summary)),
		Details: map[string]any{
			"previous": prevContent,
			"new":      payload.Content,
		},
	})

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, fmt.Sprintf("commit tx: %v", err), http.StatusInternalServerError)
		return
	}

	// Return updated statute
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statuteResponse{ID: statuteID, Content: payload.Content, UpdatedAt: updatedAt.Format(time.RFC3339)})
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func firstN(s *string) string {
	if s == nil {
		return ""
	}
	if len(*s) > 200 {
		return (*s)[:200]
	}
	return *s
}

func roleFromDBString(raw string) (model.Role, error) {
	switch raw {
	case "OWNER":
		return model.RoleOwner, nil
	case "ADMIN":
		return model.RoleAdmin, nil
	case "MODERATOR":
		return model.RoleModerator, nil
	case "EDITOR":
		return model.RoleEditor, nil
	default:
		return "", fmt.Errorf("unknown role")
	}
}

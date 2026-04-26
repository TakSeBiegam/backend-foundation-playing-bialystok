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

type privacyPolicyResponse struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	UpdatedAt string `json:"updatedAt"`
}

type privacyPolicyVersionResponse struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Summary   *string `json:"summary"`
	AuthorID  *string `json:"authorId"`
	CreatedAt string  `json:"createdAt"`
}

// PrivacyPolicyHandler returns an http.Handler that exposes GET (current privacy policy)
// and POST (update privacy policy) endpoints. POST requires X-User-Role header to be
// ADMIN or OWNER. Uses the provided pgxpool for DB access.
func PrivacyPolicyHandler(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetPrivacyPolicy(w, r, pool)
		case http.MethodPost:
			handlePostPrivacyPolicy(w, r, pool)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func PrivacyPolicyVersionsHandler(pool *pgxpool.Pool) http.Handler {
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
            FROM privacy_policy_versions
            ORDER BY created_at DESC
            LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("query privacy policy versions: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var out []privacyPolicyVersionResponse
		for rows.Next() {
			var v privacyPolicyVersionResponse
			var authorID *string
			var summary *string
			var createdAt time.Time
			if err := rows.Scan(&v.ID, &v.Content, &summary, &authorID, &createdAt); err != nil {
				http.Error(w, fmt.Sprintf("scan privacy policy version: %v", err), http.StatusInternalServerError)
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

func handleGetPrivacyPolicy(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	ctx := r.Context()

	var id string
	var content string
	var updatedAt time.Time

	err := pool.QueryRow(ctx, `SELECT id, content, updated_at FROM privacy_policies LIMIT 1`).Scan(&id, &content, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(privacyPolicyResponse{ID: "", Content: "", UpdatedAt: ""})
			return
		}

		http.Error(w, fmt.Sprintf("query privacy policy: %v", err), http.StatusInternalServerError)
		return
	}

	resp := privacyPolicyResponse{ID: id, Content: content, UpdatedAt: updatedAt.Format(time.RFC3339)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handlePostPrivacyPolicy(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
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

	// Ensure there is a privacy_policies row
	var policyID string
	var prevContent string
	err = tx.QueryRow(ctx, `SELECT id, content FROM privacy_policies LIMIT 1`).Scan(&policyID, &prevContent)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, fmt.Sprintf("query privacy policy: %v", err), http.StatusInternalServerError)
			return
		}

		prevContent = ""
		err = tx.QueryRow(ctx, `INSERT INTO privacy_policies (content) VALUES ('') RETURNING id`).Scan(&policyID)
		if err != nil {
			http.Error(w, fmt.Sprintf("insert privacy policy: %v", err), http.StatusInternalServerError)
			return
		}
	}

	var updatedAt time.Time

	// Insert version with the saved content so history reflects the actual saved revision.
	authorID := r.Header.Get("X-User-Id")
	_, err = tx.Exec(ctx, `INSERT INTO privacy_policy_versions (privacy_policy_id, content, summary, author_user_id) VALUES ($1,$2,$3,$4)`, policyID, payload.Content, payload.Summary, nullString(authorID))
	if err != nil {
		http.Error(w, fmt.Sprintf("insert privacy policy version: %v", err), http.StatusInternalServerError)
		return
	}

	// Update privacy_policies row
	err = tx.QueryRow(ctx, `UPDATE privacy_policies SET content = $1, updated_at = NOW() WHERE id = $2 RETURNING updated_at`, payload.Content, policyID).Scan(&updatedAt)
	if err != nil {
		http.Error(w, fmt.Sprintf("update privacy policy: %v", err), http.StatusInternalServerError)
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
		Resource:    model.AdminResource("PRIVACY_POLICY"),
		Action:      model.AuditActionUpdate,
		ResourceID:  &policyID,
		Summary:     fmt.Sprintf("Aktualizacja polityki prywatnosci: %s", firstN(payload.Summary)),
		Details: map[string]any{
			"previous": prevContent,
			"new":      payload.Content,
		},
	})

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, fmt.Sprintf("commit tx: %v", err), http.StatusInternalServerError)
		return
	}

	// Return updated privacy policy
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(privacyPolicyResponse{ID: policyID, Content: payload.Content, UpdatedAt: updatedAt.Format(time.RFC3339)})
}

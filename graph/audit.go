package graph

import (
	"backend/graph/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type auditExec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type auditLogEntry struct {
	ActorUserID *string
	ActorRole   *model.Role
	Resource    model.AdminResource
	Action      model.AuditAction
	ResourceID  *string
	Summary     string
	Details     map[string]any
}

func auditActorFromContext(ctx context.Context) (*string, *model.Role) {
	var actorUserID *string
	if value, err := currentUserIDFromContext(ctx); err == nil {
		actorUserID = &value
	}

	var actorRole *model.Role
	if value, err := currentRoleFromContext(ctx); err == nil {
		actorRole = &value
	}

	return actorUserID, actorRole
}

func auditSnapshotLabel(snapshot map[string]any, fallback string, keys ...string) string {
	for _, key := range keys {
		value, ok := snapshot[key]
		if !ok {
			continue
		}

		text := strings.TrimSpace(fmt.Sprintf("%v", value))
		if text != "" && text != "<nil>" {
			return text
		}
	}

	return strings.TrimSpace(fallback)
}

func sanitizeAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for key, nestedValue := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if normalizedKey == "password" || normalizedKey == "password_hash" {
				continue
			}

			cleaned[key] = sanitizeAuditValue(nestedValue)
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(typed))
		for _, nestedValue := range typed {
			cleaned = append(cleaned, sanitizeAuditValue(nestedValue))
		}
		return cleaned
	default:
		return value
	}
}

func sanitizeAuditDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}

	sanitized, _ := sanitizeAuditValue(details).(map[string]any)
	if sanitized == nil {
		return map[string]any{}
	}

	return sanitized
}

func insertAuditLogWithExec(
	ctx context.Context,
	exec auditExec,
	entry auditLogEntry,
) error {
	detailsPayload, err := json.Marshal(sanitizeAuditDetails(entry.Details))
	if err != nil {
		return fmt.Errorf("marshal audit details: %w", err)
	}

	var actorRole *string
	if entry.ActorRole != nil {
		value := roleToDB(*entry.ActorRole)
		actorRole = &value
	}

	_, err = exec.Exec(ctx,
		`INSERT INTO audit_logs (
			actor_user_id,
			actor_role,
			resource,
			action,
			resource_id,
			summary,
			details
		)
		VALUES ((SELECT id FROM users WHERE id::text = $1),$2,$3,$4,$5,$6,$7::jsonb)`,
		entry.ActorUserID,
		actorRole,
		string(entry.Resource),
		string(entry.Action),
		entry.ResourceID,
		strings.TrimSpace(entry.Summary),
		string(detailsPayload),
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func (r *Resolver) insertAuditLog(ctx context.Context, entry auditLogEntry) error {
	return insertAuditLogWithExec(ctx, r.DB, entry)
}

func fetchRowSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	table string,
	id string,
) (map[string]any, error) {
	safeTableName := strings.TrimSpace(table)
	switch safeTableName {
	case "users", "events", "partners", "offer_blocks", "board_games", "contact_submissions":
	default:
		return nil, fmt.Errorf("unsupported audit snapshot table: %s", safeTableName)
	}

	query := fmt.Sprintf(
		`SELECT to_jsonb(t) FROM (SELECT * FROM %s WHERE id = $1) AS t`,
		safeTableName,
	)

	var raw []byte
	err := tx.QueryRow(ctx, query, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("fetch %s snapshot: %w", safeTableName, err)
	}

	if len(raw) == 0 {
		return nil, nil
	}

	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, fmt.Errorf("decode %s snapshot: %w", safeTableName, err)
	}

	return sanitizeAuditDetails(snapshot), nil
}

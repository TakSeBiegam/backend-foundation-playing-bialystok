package graph

import (
	"backend/graph/model"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Resolver) getUserByID(ctx context.Context, id string) (*model.User, error) {
	row := r.DB.QueryRow(ctx,
		`SELECT id, email, username, role, created_at FROM users WHERE id = $1`,
		id,
	)

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

func (r *Resolver) getUserByEmail(ctx context.Context, email string) (*model.User, error) {
	normalizedEmail := strings.TrimSpace(email)
	if normalizedEmail == "" {
		return nil, nil
	}

	row := r.DB.QueryRow(ctx,
		`SELECT id, email, username, role, created_at
		 FROM users
		 WHERE LOWER(COALESCE(email, '')) = LOWER($1)
		 LIMIT 1`,
		normalizedEmail,
	)

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

func (r *Resolver) getUserByUsername(ctx context.Context, username string) (*model.User, error) {
	normalizedUsername := strings.TrimSpace(username)
	if normalizedUsername == "" {
		return nil, nil
	}

	row := r.DB.QueryRow(ctx,
		`SELECT id, email, username, role, created_at
		 FROM users
		 WHERE LOWER(COALESCE(username, '')) = LOWER($1)
		 LIMIT 1`,
		normalizedUsername,
	)

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

func (r *Resolver) resolveContextUser(ctx context.Context) (*model.User, error) {
	if callerID, _ := ctx.Value("callerId").(string); strings.TrimSpace(callerID) != "" {
		user, err := r.getUserByID(ctx, callerID)
		if err != nil {
			return nil, err
		}
		if user != nil {
			return user, nil
		}
	}

	callerEmail, _ := ctx.Value("callerEmail").(string)
	if user, err := r.getUserByEmail(ctx, callerEmail); err != nil {
		return nil, err
	} else if user != nil {
		return user, nil
	}

	callerName, _ := ctx.Value("callerName").(string)
	if strings.TrimSpace(callerName) == "" || strings.EqualFold(callerName, callerEmail) {
		return nil, nil
	}

	return r.getUserByUsername(ctx, callerName)
}

func (r *Resolver) resolveActingUser(ctx context.Context, preferredID string) (*model.User, error) {
	user, err := r.resolveContextUser(ctx)
	if err != nil || user != nil {
		return user, err
	}

	if strings.TrimSpace(preferredID) == "" {
		return nil, nil
	}

	return r.getUserByID(ctx, preferredID)
}

func (r *Resolver) getContactSubmissionByID(ctx context.Context, id string, withNotes bool) (*model.ContactSubmission, error) {
	var item model.ContactSubmission
	var readAt *time.Time
	var readByUserID *string
	var createdAt, updatedAt time.Time
	var lastNoteAt *time.Time

	err := r.DB.QueryRow(ctx,
		`SELECT id, first_name, last_name, phone, message, is_read, read_at, read_by_user_id, archived, last_note_at, created_at, updated_at
		 FROM contact_submissions
		 WHERE id = $1`,
		id,
	).Scan(
		&item.ID,
		&item.FirstName,
		&item.LastName,
		&item.Phone,
		&item.Message,
		&item.IsRead,
		&readAt,
		&readByUserID,
		&item.Archived,
		&lastNoteAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("contact submission not found")
		}
		return nil, fmt.Errorf("get contact submission: %w", err)
	}

	if readAt != nil {
		t := readAt.Format(time.RFC3339)
		item.ReadAt = &t
	}

	if readByUserID != nil {
		reader, err := r.getUserByID(ctx, *readByUserID)
		if err != nil {
			return nil, err
		}
		item.ReadBy = reader
	}

	item.CreatedAt = createdAt.Format(time.RFC3339)
	item.UpdatedAt = updatedAt.Format(time.RFC3339)
	if lastNoteAt != nil {
		s := lastNoteAt.Format(time.RFC3339)
		item.LastNoteAt = &s
	}

	if withNotes {
		notes, err := r.loadContactSubmissionNotes(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		item.Notes = notes
	} else {
		item.Notes = []*model.ContactSubmissionNote{}
	}

	return &item, nil
}

func (r *Resolver) loadContactSubmissionNotes(ctx context.Context, submissionID string) ([]*model.ContactSubmissionNote, error) {
	rows, err := r.DB.Query(ctx,
		`SELECT id, submission_id, note, author_user_id, created_at, updated_at
		 FROM contact_submission_notes
		 WHERE submission_id = $1
		 ORDER BY created_at DESC`,
		submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list contact submission notes: %w", err)
	}
	defer rows.Close()

	notes := make([]*model.ContactSubmissionNote, 0)
	for rows.Next() {
		var note model.ContactSubmissionNote
		var authorUserID string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&note.ID,
			&note.SubmissionID,
			&note.Note,
			&authorUserID,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan contact submission note: %w", err)
		}

		author, err := r.getUserByID(ctx, authorUserID)
		if err != nil {
			return nil, err
		}
		if author == nil {
			return nil, fmt.Errorf("note author not found")
		}

		note.CreatedAt = createdAt.Format(time.RFC3339)
		note.UpdatedAt = updatedAt.Format(time.RFC3339)
		note.Author = author

		notes = append(notes, &note)
	}

	return notes, nil
}

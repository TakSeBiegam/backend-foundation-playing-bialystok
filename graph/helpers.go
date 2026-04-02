package graph

import (
	"backend/graph/model"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

func roleFromDB(r string) model.Role {
	switch strings.ToLower(r) {
	case "admin":
		return model.RoleAdmin
	case "moderator":
		return model.RoleModerator
	case "owner":
		return model.RoleOwner
	default:
		return model.RoleEditor
	}
}

func roleToDB(r model.Role) string {
	return strings.ToLower(string(r))
}

const passwordCharset = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

func randomPassword(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = passwordCharset[rand.Intn(len(passwordCharset))]
	}
	return string(b)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(row rowScanner) (*model.Event, error) {
	var e model.Event
	var date time.Time
	var createdAt time.Time
	if err := row.Scan(&e.ID, &e.Title, &e.Description, &date, &e.Location, &e.Time, &e.FacebookURL, &e.ImageURL, &createdAt); err != nil {
		return nil, fmt.Errorf("scan event: %w", err)
	}
	e.Date = date.Format("2006-01-02")
	e.CreatedAt = createdAt.Format(time.RFC3339)
	return &e, nil
}

type pgxRows interface {
	Scan(dest ...any) error
}

func cleanStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}

	return cleaned
}

func normalizeRequiredString(value string) string {
	return strings.TrimSpace(value)
}

func normalizeNullableString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func scanEventRow(row pgxRows) (*model.Event, error) {
	return scanEvent(row)
}

func scanPartner(row rowScanner) (*model.Partner, error) {
	var p model.Partner
	if err := row.Scan(&p.ID, &p.Name, &p.LogoURL, &p.WebsiteURL, &p.Description); err != nil {
		return nil, fmt.Errorf("scan partner: %w", err)
	}
	return &p, nil
}

func scanPartnerRow(row pgxRows) (*model.Partner, error) {
	return scanPartner(row)
}

func scanOfferBlock(row rowScanner) (*model.OfferBlock, error) {
	var block model.OfferBlock
	var createdAt, updatedAt time.Time
	if err := row.Scan(
		&block.ID,
		&block.Section,
		&block.BlockType,
		&block.Badge,
		&block.Title,
		&block.Subtitle,
		&block.Content,
		&block.Items,
		&block.Highlight,
		&block.ImageURL,
		&block.ImageAlt,
		&block.CtaLabel,
		&block.CtaHref,
		&block.IsFeatured,
		&block.Order,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan offer block: %w", err)
	}

	block.Items = cleanStringSlice(block.Items)
	block.CreatedAt = createdAt.Format(time.RFC3339)
	block.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &block, nil
}

func scanOfferBlockRow(row pgxRows) (*model.OfferBlock, error) {
	return scanOfferBlock(row)
}

func scanUser(row rowScanner) (*model.User, error) {
	var u model.User
	var dbRole string
	var createdAt time.Time
	if err := row.Scan(&u.ID, &u.Email, &u.Username, &dbRole, &createdAt); err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.Role = roleFromDB(dbRole)
	u.CreatedAt = createdAt.Format(time.RFC3339)
	return &u, nil
}

func scanUserRow(row pgxRows) (*model.User, error) {
	return scanUser(row)
}

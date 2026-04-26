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

func normalizeOptionalString(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func normalizeSignInIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeNullableBoardGameDifficulty(value *model.BoardGameDifficulty) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(boardGameDifficultyToDB(*value))
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func normalizeOptionalBoardGameDifficulty(value *model.BoardGameDifficulty) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(boardGameDifficultyToDB(*value))
}

const (
	boardGameDifficultyDBImprezowa = "Imprezowa"
	boardGameDifficultyDBLatwa     = "Łatwa"
	boardGameDifficultyDBSredni    = "Średni"
	boardGameDifficultyDBEkspercka = "Ekspercka"
)

var boardGameCatalogDifficultyAliases = map[string]string{
	"imprezowa":              model.BoardGameDifficultyImprezowa.String(),
	"imprezowy":              model.BoardGameDifficultyImprezowa.String(),
	"latwa":                  model.BoardGameDifficultyLatwa.String(),
	"łatwa":                  model.BoardGameDifficultyLatwa.String(),
	"lekki":                  model.BoardGameDifficultyLatwa.String(),
	"łatwy":                  model.BoardGameDifficultyLatwa.String(),
	"latwy":                  model.BoardGameDifficultyLatwa.String(),
	"strategiczny":           model.BoardGameDifficultySredni.String(),
	"strategiczna":           model.BoardGameDifficultySredni.String(),
	"srednia":                model.BoardGameDifficultySredni.String(),
	"średnia":                model.BoardGameDifficultySredni.String(),
	"sredni":                 model.BoardGameDifficultySredni.String(),
	"średni":                 model.BoardGameDifficultySredni.String(),
	"srednia strategiczna":   model.BoardGameDifficultySredni.String(),
	"średnia strategiczna":   model.BoardGameDifficultySredni.String(),
	"srednia (strategiczna)": model.BoardGameDifficultySredni.String(),
	"średnia (strategiczna)": model.BoardGameDifficultySredni.String(),
	"srednia_strategiczna":   model.BoardGameDifficultySredni.String(),
	"ekspercki":              model.BoardGameDifficultyEkspercka.String(),
	"ekspercka":              model.BoardGameDifficultyEkspercka.String(),
	"eskpercka":              model.BoardGameDifficultyEkspercka.String(),
	"ekspert":                model.BoardGameDifficultyEkspercka.String(),
	"expert":                 model.BoardGameDifficultyEkspercka.String(),
	"zaawansowany":           model.BoardGameDifficultyEkspercka.String(),
}

func normalizeBoardGameCatalogDifficultyText(value *string) string {
	if value == nil {
		return ""
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return ""
	}

	if mapped, ok := boardGameCatalogDifficultyAliases[strings.ToLower(trimmed)]; ok {
		return mapped
	}

	return trimmed
}

func normalizeBoardGameCatalogDifficultyList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, rawValue := range values {
		value := rawValue
		normalizedValue := normalizeBoardGameCatalogDifficultyText(&value)
		if normalizedValue == "" {
			continue
		}

		key := strings.ToLower(normalizedValue)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		normalized = append(normalized, normalizedValue)
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func boardGameDifficultyToDB(value model.BoardGameDifficulty) string {
	switch value {
	case model.BoardGameDifficultyImprezowa:
		return boardGameDifficultyDBImprezowa
	case model.BoardGameDifficultyLatwa:
		return boardGameDifficultyDBLatwa
	case model.BoardGameDifficultySredni:
		return boardGameDifficultyDBSredni
	case model.BoardGameDifficultyEkspercka:
		return boardGameDifficultyDBEkspercka
	default:
		return strings.TrimSpace(string(value))
	}
}

func boardGameDifficultyFromDB(value string) *model.BoardGameDifficulty {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "imprezowa":
		mapped := model.BoardGameDifficultyImprezowa
		return &mapped
	case "łatwa", "latwa":
		mapped := model.BoardGameDifficultyLatwa
		return &mapped
	case "średni", "sredni":
		mapped := model.BoardGameDifficultySredni
		return &mapped
	case "ekspercka":
		mapped := model.BoardGameDifficultyEkspercka
		return &mapped
	default:
		return nil
	}
}

func boardGameDifficultyListFromDB(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	mapped := make([]string, 0, len(values))
	for _, value := range values {
		if difficulty := boardGameDifficultyFromDB(value); difficulty != nil {
			mapped = append(mapped, boardGameDifficultyToDB(*difficulty))
		}
	}

	return mapped
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

func scanBoardGame(row rowScanner) (*model.BoardGame, error) {
	var game model.BoardGame
	var difficulty *string
	var createdAt, updatedAt time.Time
	if err := row.Scan(
		&game.ID,
		&game.Title,
		&game.Description,
		&game.PlayerBucket,
		&game.PlayTime,
		&game.Category,
		&difficulty,
		&game.ImageURL,
		&game.ImageAlt,
		&game.Order,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan board game: %w", err)
	}

	game.CreatedAt = createdAt.Format(time.RFC3339)
	game.UpdatedAt = updatedAt.Format(time.RFC3339)
	if difficulty != nil {
		game.Difficulty = boardGameDifficultyFromDB(*difficulty)
	}
	return &game, nil
}

func scanBoardGameRow(row pgxRows) (*model.BoardGame, error) {
	return scanBoardGame(row)
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

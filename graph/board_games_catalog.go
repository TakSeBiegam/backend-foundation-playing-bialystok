package graph

import (
	"backend/graph/model"
	"context"
	"fmt"
	"strings"
)

const (
	defaultBoardGameCatalogLimit int32 = 9
	maxBoardGameCatalogLimit     int32 = 60
)

type boardGameCatalogFilter struct {
	search       string
	playerBucket string
	category     string
	difficulties []string
}

func normalizeBoardGameCatalogFilterDifficulties(single *string, multiple []string) []string {
	normalized := normalizeBoardGameCatalogDifficultyList(multiple)
	singleValue := normalizeBoardGameCatalogDifficultyText(single)
	if singleValue == "" {
		return normalized
	}

	for _, existingValue := range normalized {
		if strings.EqualFold(existingValue, singleValue) {
			return normalized
		}
	}

	return append([]string{singleValue}, normalized...)
}

func normalizeBoardGameCatalogInput(input *model.BoardGameCatalogInput) (boardGameCatalogFilter, model.BoardGameSortMode, int32, int32) {
	filter := boardGameCatalogFilter{}
	sortMode := model.BoardGameSortModeAz
	limit := defaultBoardGameCatalogLimit
	offset := int32(0)

	if input == nil {
		return filter, sortMode, limit, offset
	}

	if input.Filter != nil {
		filter = boardGameCatalogFilter{
			search:       normalizeOptionalString(input.Filter.Search),
			playerBucket: normalizeOptionalString(input.Filter.PlayerBucket),
			category:     normalizeOptionalString(input.Filter.Category),
			difficulties: normalizeBoardGameCatalogFilterDifficulties(
				input.Filter.Difficulty,
				input.Filter.Difficulties,
			),
		}
	}

	if input.Sort != nil {
		sortMode = *input.Sort
	}

	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit <= 0 {
		limit = defaultBoardGameCatalogLimit
	}
	if limit > maxBoardGameCatalogLimit {
		limit = maxBoardGameCatalogLimit
	}

	if input.Offset != nil && *input.Offset > 0 {
		offset = *input.Offset
	}

	return filter, sortMode, limit, offset
}

func buildBoardGameCatalogWhere(filter boardGameCatalogFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 6)
	argIndex := 1

	if filter.search != "" {
		clauses = append(clauses, fmt.Sprintf("title ILIKE $%d", argIndex))
		args = append(args, "%"+filter.search+"%")
		argIndex++
	}

	if filter.playerBucket != "" {
		clauses = append(clauses, fmt.Sprintf("LOWER(player_bucket) = LOWER($%d)", argIndex))
		args = append(args, filter.playerBucket)
		argIndex++
	}

	if filter.category != "" {
		clauses = append(clauses, fmt.Sprintf("LOWER(category) = LOWER($%d)", argIndex))
		args = append(args, filter.category)
		argIndex++
	}

	if len(filter.difficulties) > 0 {
		difficultyClauses := make([]string, 0, len(filter.difficulties))

		for _, difficulty := range filter.difficulties {
			difficultyClauses = append(
				difficultyClauses,
				fmt.Sprintf("LOWER(difficulty::text) = LOWER($%d)", argIndex),
			)
			args = append(args, difficulty)
			argIndex++
		}

		clauses = append(clauses, "("+strings.Join(difficultyClauses, " OR ")+")")
	}

	if len(clauses) == 0 {
		return "", args
	}

	return " WHERE " + strings.Join(clauses, " AND "), args
}

func buildBoardGameCatalogOrder(sortMode model.BoardGameSortMode) string {
	switch sortMode {
	case model.BoardGameSortModeZa:
		return " ORDER BY LOWER(title) DESC, created_at DESC"
	case model.BoardGameSortModePlayers:
		return ` ORDER BY CASE player_bucket
			WHEN '1-2' THEN 0
			WHEN '2-4' THEN 1
			WHEN '4+' THEN 2
			ELSE 99
		END, LOWER(title), created_at`
	case model.BoardGameSortModeOrder:
		return " ORDER BY display_order, LOWER(title), created_at"
	default:
		return " ORDER BY LOWER(title), created_at"
	}
}

func (r *queryResolver) listBoardGameFacet(ctx context.Context, column string) ([]string, error) {
	allowedColumns := map[string]bool{
		"category":   true,
		"difficulty": true,
	}
	if !allowedColumns[column] {
		return nil, fmt.Errorf("unsupported board game facet: %s", column)
	}

	query := fmt.Sprintf(
		`SELECT DISTINCT BTRIM(%s::text) AS value
		 FROM board_games
		 WHERE %s IS NOT NULL AND BTRIM(%s::text) <> ''
		 ORDER BY value`,
		column,
		column,
		column,
	)

	rows, err := r.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list board game facet %s: %w", column, err)
	}
	defer rows.Close()

	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("scan board game facet %s: %w", column, err)
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}

	return values, nil
}

func (r *queryResolver) resolveBoardGamesCatalog(ctx context.Context, input *model.BoardGameCatalogInput) (*model.BoardGameCatalogPage, error) {
	filter, sortMode, limit, offset := normalizeBoardGameCatalogInput(input)
	whereClause, args := buildBoardGameCatalogWhere(filter)

	countQuery := `SELECT COUNT(*) FROM board_games` + whereClause
	var totalCount int32
	if err := r.DB.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count board games catalog: %w", err)
	}

	itemsQuery := `SELECT
			id,
			title,
			description,
			player_bucket,
			play_time,
			category,
			difficulty,
			image_url,
			image_alt,
			display_order,
			created_at,
			updated_at
		 FROM board_games` + whereClause + buildBoardGameCatalogOrder(sortMode) + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

	rows, err := r.DB.Query(ctx, itemsQuery, append(args, limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("list board games catalog: %w", err)
	}
	defer rows.Close()

	items := make([]*model.BoardGame, 0, limit)
	for rows.Next() {
		item, err := scanBoardGameRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	categories, err := r.listBoardGameFacet(ctx, "category")
	if err != nil {
		return nil, err
	}

	difficulties, err := r.listBoardGameFacet(ctx, "difficulty")
	if err != nil {
		return nil, err
	}

	var catalogTotalCount int32
	var catalogWithImagesCount int32
	if err := r.DB.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE image_url IS NOT NULL AND BTRIM(image_url) <> '')
		 FROM board_games`,
	).Scan(&catalogTotalCount, &catalogWithImagesCount); err != nil {
		return nil, fmt.Errorf("count board games catalog summary: %w", err)
	}

	hasMore := offset+int32(len(items)) < totalCount
	var nextOffset *int32
	if hasMore {
		value := offset + int32(len(items))
		nextOffset = &value
	}

	return &model.BoardGameCatalogPage{
		Items:                  items,
		TotalCount:             totalCount,
		HasMore:                hasMore,
		NextOffset:             nextOffset,
		Categories:             categories,
		Difficulties:           boardGameDifficultyListFromDB(difficulties),
		CatalogTotalCount:      catalogTotalCount,
		CatalogWithImagesCount: catalogWithImagesCount,
	}, nil
}

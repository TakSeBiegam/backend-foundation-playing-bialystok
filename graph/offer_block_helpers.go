package graph

import (
	"backend/graph/model"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Resolver) getOfferBlockByID(ctx context.Context, id string) (*model.OfferBlock, error) {
	row := r.DB.QueryRow(ctx,
		`SELECT
			id,
			page_key,
			category_key,
			section,
			block_type,
			badge,
			title,
			subtitle,
			content,
			items,
			highlight,
			image_url,
			image_alt,
			cta_label,
			cta_href,
			is_featured,
			display_order,
			created_at,
			updated_at
		 FROM offer_blocks
		 WHERE id = $1`,
		id,
	)

	block, err := scanOfferBlock(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return block, nil
}

func (r *Resolver) getOfferBlockPageKeyByID(ctx context.Context, id string) (string, error) {
	row := r.DB.QueryRow(ctx,
		`SELECT page_key
		 FROM offer_blocks
		 WHERE id = $1`,
		id,
	)

	var pageKey string
	if err := row.Scan(&pageKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("offer block not found")
		}

		return "", fmt.Errorf("get offer block page key: %w", err)
	}

	return normalizeOfferBlockPageKey(pageKey), nil
}

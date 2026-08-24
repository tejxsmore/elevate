package repository

import (
	"context"

	"github.com/google/uuid"

	"elevate/internal/database"
	"elevate/internal/models"
)

type LeadRepo struct {
	db *database.DB
}

func NewLeadRepo(db *database.DB) *LeadRepo {
	return &LeadRepo{db: db}
}

func (r *LeadRepo) List(
	ctx context.Context,
	limit,
	offset int,
) ([]models.Lead, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			id,
			phone_e164,
			name,
			preferred_language,
			source,
			created_at,
			updated_at
		FROM leads
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
		`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]models.Lead, 0, limit)

	for rows.Next() {
		var l models.Lead

		if err := rows.Scan(
			&l.ID,
			&l.PhoneE164,
			&l.Name,
			&l.PreferredLanguage,
			&l.Source,
			&l.CreatedAt,
			&l.UpdatedAt,
		); err != nil {
			return nil, err
		}

		leads = append(leads, l)
	}

	return leads, rows.Err()
}

func (r *LeadRepo) Upsert(
	ctx context.Context,
	phoneE164 string,
	name *string,
	lang models.LanguageCode,
	source *string,
) (models.Lead, error) {
	var l models.Lead

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO leads (
			phone_e164,
			name,
			preferred_language,
			source
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (phone_e164)
		DO UPDATE SET
			name = COALESCE(EXCLUDED.name, leads.name),
			preferred_language = CASE
				WHEN EXCLUDED.preferred_language = 'unknown'
					THEN leads.preferred_language
				ELSE EXCLUDED.preferred_language
			END,
			source = COALESCE(EXCLUDED.source, leads.source)
		RETURNING
			id,
			phone_e164,
			name,
			preferred_language,
			source,
			created_at,
			updated_at
		`,
		phoneE164,
		name,
		lang,
		source,
	).Scan(
		&l.ID,
		&l.PhoneE164,
		&l.Name,
		&l.PreferredLanguage,
		&l.Source,
		&l.CreatedAt,
		&l.UpdatedAt,
	)

	return l, err
}

func (r *LeadRepo) Get(
	ctx context.Context,
	id uuid.UUID,
) (models.Lead, error) {
	var l models.Lead

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			phone_e164,
			name,
			preferred_language,
			source,
			created_at,
			updated_at
		FROM leads
		WHERE id = $1
		`,
		id,
	).Scan(
		&l.ID,
		&l.PhoneE164,
		&l.Name,
		&l.PreferredLanguage,
		&l.Source,
		&l.CreatedAt,
		&l.UpdatedAt,
	)

	return l, err
}

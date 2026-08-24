package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/database"
	"elevate/internal/models"
)

type DiscoveryRepo struct {
	db *database.DB
}

func NewDiscoveryRepo(
	db *database.DB,
) *DiscoveryRepo {
	return &DiscoveryRepo{
		db: db,
	}
}

func (r *DiscoveryRepo) EnsureProfile(
	ctx context.Context,
	callID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		INSERT INTO discovery_profiles (
			call_id
		)
		VALUES ($1)
		ON CONFLICT (call_id)
		DO NOTHING
		`,
		callID,
	)

	return err
}

func (r *DiscoveryRepo) GetByCallID(
	ctx context.Context,
	callID uuid.UUID,
) (models.DiscoveryProfile, error) {
	return r.getByCallID(
		ctx,
		r.db.Pool,
		callID,
		false,
	)
}

func (r *DiscoveryRepo) getByCallID(
	ctx context.Context,
	queryer interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	},
	callID uuid.UUID,
	forUpdate bool,
) (models.DiscoveryProfile, error) {
	lockClause := ""

	if forUpdate {
		lockClause = " FOR UPDATE"
	}

	var profile models.DiscoveryProfile

	err := queryer.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			business_niche,
			products_sold,
			product_count_estimate,
			budget_range,
			budget_raw_text,
			timeline,
			timeline_raw_text,
			features_requested,
			extra_notes,
			updated_at
		FROM discovery_profiles
		WHERE call_id = $1
		`+lockClause,
		callID,
	).Scan(
		&profile.ID,
		&profile.CallID,
		&profile.BusinessNiche,
		&profile.ProductsSold,
		&profile.ProductCountEstimate,
		&profile.BudgetRange,
		&profile.BudgetRawText,
		&profile.Timeline,
		&profile.TimelineRawText,
		&profile.FeaturesRequested,
		&profile.ExtraNotes,
		&profile.UpdatedAt,
	)

	return profile, err
}

func (r *DiscoveryRepo) GetOrCreate(
	ctx context.Context,
	callID uuid.UUID,
) (models.DiscoveryProfile, error) {
	if err := r.EnsureProfile(
		ctx,
		callID,
	); err != nil {
		return models.DiscoveryProfile{}, err
	}

	return r.GetByCallID(
		ctx,
		callID,
	)
}

type DiscoveryUpdate struct {
	BusinessNiche        *string
	ProductsSold         *string
	ProductCountEstimate *string
	BudgetRange          *string
	BudgetRawText        *string
	Timeline             *string
	TimelineRawText      *string
	FeaturesRequested    []string
	ExtraNotes           *string
}

func (r *DiscoveryRepo) Upsert(
	ctx context.Context,
	callID uuid.UUID,
	update DiscoveryUpdate,
) (models.DiscoveryProfile, error) {
	tx, err := r.db.Pool.Begin(
		ctx,
	)
	if err != nil {
		return models.DiscoveryProfile{}, err
	}

	defer func() {
		_ = tx.Rollback(
			ctx,
		)
	}()

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO discovery_profiles (
			call_id
		)
		VALUES ($1)
		ON CONFLICT (call_id)
		DO NOTHING
		`,
		callID,
	)
	if err != nil {
		return models.DiscoveryProfile{}, err
	}

	current, err := r.getByCallID(
		ctx,
		tx,
		callID,
		true,
	)
	if err != nil {
		return models.DiscoveryProfile{}, err
	}

	businessNiche := chooseString(
		current.BusinessNiche,
		update.BusinessNiche,
	)

	productsSold := chooseString(
		current.ProductsSold,
		update.ProductsSold,
	)

	productCount := chooseString(
		current.ProductCountEstimate,
		update.ProductCountEstimate,
	)

	budgetRange := chooseString(
		current.BudgetRange,
		update.BudgetRange,
	)

	budgetRaw := chooseString(
		current.BudgetRawText,
		update.BudgetRawText,
	)

	timeline := chooseString(
		current.Timeline,
		update.Timeline,
	)

	timelineRaw := chooseString(
		current.TimelineRawText,
		update.TimelineRawText,
	)

	features := mergeFeatures(
		current.FeaturesRequested,
		update.FeaturesRequested,
	)

	extraNotes := chooseString(
		current.ExtraNotes,
		update.ExtraNotes,
	)

	featuresJSON, err := json.Marshal(
		features,
	)
	if err != nil {
		return models.DiscoveryProfile{}, err
	}

	var profile models.DiscoveryProfile

	err = tx.QueryRow(
		ctx,
		`
		UPDATE discovery_profiles
		SET
			business_niche = $2,
			products_sold = $3,
			product_count_estimate = $4,
			budget_range = $5,
			budget_raw_text = $6,
			timeline = $7,
			timeline_raw_text = $8,
			features_requested = $9,
			extra_notes = $10,
			updated_at = now()
		WHERE call_id = $1
		RETURNING
			id,
			call_id,
			business_niche,
			products_sold,
			product_count_estimate,
			budget_range,
			budget_raw_text,
			timeline,
			timeline_raw_text,
			features_requested,
			extra_notes,
			updated_at
		`,
		callID,
		businessNiche,
		productsSold,
		productCount,
		budgetRange,
		budgetRaw,
		timeline,
		timelineRaw,
		featuresJSON,
		extraNotes,
	).Scan(
		&profile.ID,
		&profile.CallID,
		&profile.BusinessNiche,
		&profile.ProductsSold,
		&profile.ProductCountEstimate,
		&profile.BudgetRange,
		&profile.BudgetRawText,
		&profile.Timeline,
		&profile.TimelineRawText,
		&profile.FeaturesRequested,
		&profile.ExtraNotes,
		&profile.UpdatedAt,
	)
	if err != nil {
		return models.DiscoveryProfile{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.DiscoveryProfile{}, err
	}

	return profile, nil
}

func (r *DiscoveryRepo) AddBarrier(
	ctx context.Context,
	callID uuid.UUID,
	barrierType models.BarrierType,
	detail string,
	rawQuote string,
) error {
	var detailValue any
	var quoteValue any

	detail = strings.TrimSpace(detail)
	rawQuote = strings.TrimSpace(rawQuote)

	if detail != "" {
		detailValue = detail
	}

	if rawQuote != "" {
		quoteValue = rawQuote
	}

	_, err := r.db.Pool.Exec(
		ctx,
		`
		INSERT INTO lead_barriers (
			call_id,
			barrier_type,
			detail,
			raw_quote
		)
		VALUES ($1, $2, $3, $4)
		`,
		callID,
		barrierType,
		detailValue,
		quoteValue,
	)

	return err
}

func chooseString(
	current *string,
	incoming *string,
) *string {
	if incoming == nil {
		return current
	}

	value := strings.TrimSpace(
		*incoming,
	)

	if value == "" {
		return current
	}

	return &value
}

func mergeFeatures(
	existing models.JSONB,
	additions []string,
) []string {
	seen := make(
		map[string]struct{},
	)

	result := make(
		[]string,
		0,
	)

	var current []string

	if len(existing) > 0 {
		_ = json.Unmarshal(
			existing,
			&current,
		)
	}

	values := make(
		[]string,
		0,
		len(current)+len(additions),
	)

	values = append(
		values,
		current...,
	)

	values = append(
		values,
		additions...,
	)

	for _, value := range values {
		original := strings.TrimSpace(
			value,
		)

		if original == "" {
			continue
		}

		key := strings.ToLower(
			original,
		)

		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}

		result = append(
			result,
			original,
		)
	}

	return result
}

func IsNotFound(
	err error,
) bool {
	return errors.Is(
		err,
		pgx.ErrNoRows,
	)
}

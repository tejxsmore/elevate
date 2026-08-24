package models

import (
	"time"

	"github.com/google/uuid"
)

type DiscoveryProfile struct {
	ID                   uuid.UUID `db:"id" json:"id"`
	CallID               uuid.UUID `db:"call_id" json:"call_id"`
	BusinessNiche        *string   `db:"business_niche" json:"business_niche,omitempty"`
	ProductsSold         *string   `db:"products_sold" json:"products_sold,omitempty"`
	ProductCountEstimate *string   `db:"product_count_estimate" json:"product_count_estimate,omitempty"`
	BudgetRange          *string   `db:"budget_range" json:"budget_range,omitempty"`
	BudgetRawText        *string   `db:"budget_raw_text" json:"budget_raw_text,omitempty"`
	Timeline             *string   `db:"timeline" json:"timeline,omitempty"`
	TimelineRawText      *string   `db:"timeline_raw_text" json:"timeline_raw_text,omitempty"`
	FeaturesRequested    JSONB     `db:"features_requested" json:"features_requested"`
	ExtraNotes           *string   `db:"extra_notes" json:"extra_notes,omitempty"`
	UpdatedAt            time.Time `db:"updated_at" json:"updated_at"`
}

type LeadBarrier struct {
	ID          uuid.UUID   `db:"id" json:"id"`
	CallID      uuid.UUID   `db:"call_id" json:"call_id"`
	BarrierType BarrierType `db:"barrier_type" json:"barrier_type"`
	Detail      *string     `db:"detail" json:"detail,omitempty"`
	RawQuote    *string     `db:"raw_quote" json:"raw_quote,omitempty"`
	CreatedAt   time.Time   `db:"created_at" json:"created_at"`
}

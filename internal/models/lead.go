package models

import (
	"time"

	"github.com/google/uuid"
)

type Lead struct {
	ID                uuid.UUID    `db:"id" json:"id"`
	PhoneE164         string       `db:"phone_e164" json:"phone_e164"`
	Name              *string      `db:"name" json:"name,omitempty"`
	PreferredLanguage LanguageCode `db:"preferred_language" json:"preferred_language"`
	Source            *string      `db:"source" json:"source,omitempty"`
	CreatedAt         time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time    `db:"updated_at" json:"updated_at"`
}

type LeadProfile struct {
	LeadID               uuid.UUID  `db:"lead_id" json:"lead_id"`
	BusinessNiche        *string    `db:"business_niche" json:"business_niche,omitempty"`
	ProductsSold         *string    `db:"products_sold" json:"products_sold,omitempty"`
	ProductCountEstimate *string    `db:"product_count_estimate" json:"product_count_estimate,omitempty"`
	BudgetMin            *float64   `db:"budget_min" json:"budget_min,omitempty"`
	BudgetMax            *float64   `db:"budget_max" json:"budget_max,omitempty"`
	Currency             string     `db:"currency" json:"currency"`
	TimelineText         *string    `db:"timeline_text" json:"timeline_text,omitempty"`
	FeaturesRequested    JSONB      `db:"features_requested" json:"features_requested"`
	LastUpdatedCallID    *uuid.UUID `db:"last_updated_call_id" json:"last_updated_call_id,omitempty"`
	UpdatedAt            time.Time  `db:"updated_at" json:"updated_at"`
}

type LeadCommunicationPreference struct {
	LeadID          uuid.UUID `db:"lead_id" json:"lead_id"`
	CallsAllowed    bool      `db:"calls_allowed" json:"calls_allowed"`
	WhatsappAllowed bool      `db:"whatsapp_allowed" json:"whatsapp_allowed"`
	DoNotContact    bool      `db:"do_not_contact" json:"do_not_contact"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

package models

import (
	"time"

	"github.com/google/uuid"
)

type WhatsappMessage struct {
	ID                uuid.UUID           `db:"id" json:"id"`
	CallID            *uuid.UUID          `db:"call_id" json:"call_id,omitempty"`
	LeadID            uuid.UUID           `db:"lead_id" json:"lead_id"`
	ActionID          *uuid.UUID          `db:"action_id" json:"action_id,omitempty"`
	MessageType       WhatsappMessageType `db:"message_type" json:"message_type"`
	Provider          string              `db:"provider" json:"provider"`
	ToNumber          string              `db:"to_number" json:"to_number"`
	Body              string              `db:"body" json:"body"`
	IdempotencyKey    *string             `db:"idempotency_key" json:"idempotency_key,omitempty"`
	ProviderMessageID *string             `db:"provider_message_id" json:"provider_message_id,omitempty"`
	Status            WhatsappStatus      `db:"status" json:"status"`
	SentDuringCall    bool                `db:"sent_during_call" json:"sent_during_call"`
	SentAt            *time.Time          `db:"sent_at" json:"sent_at,omitempty"`
	DeliveredAt       *time.Time          `db:"delivered_at" json:"delivered_at,omitempty"`
	ReadAt            *time.Time          `db:"read_at" json:"read_at,omitempty"`
	ErrorMessage      *string             `db:"error_message" json:"error_message,omitempty"`
	CreatedAt         time.Time           `db:"created_at" json:"created_at"`
}

type WhatsappMessageAsset struct {
	WhatsappMessageID uuid.UUID `db:"whatsapp_message_id" json:"whatsapp_message_id"`
	AssetID           uuid.UUID `db:"asset_id" json:"asset_id"`
}

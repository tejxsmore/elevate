package models

import (
	"time"

	"github.com/google/uuid"
)

type ScheduledCallback struct {
	ID                   uuid.UUID      `db:"id" json:"id"`
	CallID               uuid.UUID      `db:"call_id" json:"call_id"`
	LeadID               uuid.UUID      `db:"lead_id" json:"lead_id"`
	RequestedTimeText    string         `db:"requested_time_text" json:"requested_time_text"`
	ScheduledFor         *time.Time     `db:"scheduled_for" json:"scheduled_for,omitempty"`
	Timezone             string         `db:"timezone" json:"timezone"`
	ResolutionConfidence *float64       `db:"resolution_confidence" json:"resolution_confidence,omitempty"`
	ResolutionSource     *string        `db:"resolution_source" json:"resolution_source,omitempty"`
	ResolvedFrom         JSONB          `db:"resolved_from" json:"resolved_from"`
	Status               CallbackStatus `db:"status" json:"status"`
	ReminderSent         bool           `db:"reminder_sent" json:"reminder_sent"`
	CallbackActionID     *uuid.UUID     `db:"callback_action_id" json:"callback_action_id,omitempty"`
	FollowUpCallID       *uuid.UUID     `db:"follow_up_call_id" json:"follow_up_call_id,omitempty"`
	CreatedAt            time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time      `db:"updated_at" json:"updated_at"`
}

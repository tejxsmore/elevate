package models

import (
	"time"

	"github.com/google/uuid"
)

type Campaign struct {
	ID                    uuid.UUID  `db:"id" json:"id"`
	Name                  string     `db:"name" json:"name"`
	SystemPrompt          *string    `db:"system_prompt" json:"system_prompt,omitempty"`
	VoiceConfig           JSONB      `db:"voice_config" json:"voice_config"`
	WhatsappTemplates     JSONB      `db:"whatsapp_templates" json:"whatsapp_templates"`
	DefaultResumeAssetID  *uuid.UUID `db:"default_resume_asset_id" json:"default_resume_asset_id,omitempty"`
	DefaultDiagramAssetID *uuid.UUID `db:"default_diagram_asset_id" json:"default_diagram_asset_id,omitempty"`
	AgentPhoneNumber      *string    `db:"agent_phone_number" json:"agent_phone_number,omitempty"`
	Active                bool       `db:"active" json:"active"`
	Version               int        `db:"version" json:"version"`
	ArchivedAt            *time.Time `db:"archived_at" json:"archived_at,omitempty"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at" json:"updated_at"`
}

type CampaignVersion struct {
	ID                    uuid.UUID  `db:"id" json:"id"`
	CampaignID            uuid.UUID  `db:"campaign_id" json:"campaign_id"`
	Version               int        `db:"version" json:"version"`
	SystemPrompt          *string    `db:"system_prompt" json:"system_prompt,omitempty"`
	VoiceConfig           JSONB      `db:"voice_config" json:"voice_config"`
	WhatsappTemplates     JSONB      `db:"whatsapp_templates" json:"whatsapp_templates"`
	DefaultResumeAssetID  *uuid.UUID `db:"default_resume_asset_id" json:"default_resume_asset_id,omitempty"`
	DefaultDiagramAssetID *uuid.UUID `db:"default_diagram_asset_id" json:"default_diagram_asset_id,omitempty"`
	AgentPhoneNumber      *string    `db:"agent_phone_number" json:"agent_phone_number,omitempty"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
}

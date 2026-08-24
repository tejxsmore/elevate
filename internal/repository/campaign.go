package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"elevate/internal/database"
	"elevate/internal/models"
)

type CampaignRepo struct {
	db *database.DB
}

func NewCampaignRepo(
	db *database.DB,
) *CampaignRepo {
	return &CampaignRepo{
		db: db,
	}
}

type CampaignSummary struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	Active           bool      `json:"active"`
	Version          int       `json:"version"`
	AgentPhoneNumber *string   `json:"agent_phone_number,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (r *CampaignRepo) List(
	ctx context.Context,
) ([]CampaignSummary, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			id,
			name,
			active,
			version,
			agent_phone_number,
			created_at,
			updated_at
		FROM campaigns
		WHERE archived_at IS NULL
		ORDER BY created_at DESC
		`,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	campaigns := make(
		[]CampaignSummary,
		0,
	)

	for rows.Next() {
		var campaign CampaignSummary

		if err := rows.Scan(
			&campaign.ID,
			&campaign.Name,
			&campaign.Active,
			&campaign.Version,
			&campaign.AgentPhoneNumber,
			&campaign.CreatedAt,
			&campaign.UpdatedAt,
		); err != nil {
			return nil, err
		}

		campaigns = append(
			campaigns,
			campaign,
		)
	}

	return campaigns, rows.Err()
}

func (r *CampaignRepo) Create(
	ctx context.Context,
	name string,
	systemPrompt *string,
	voiceConfig json.RawMessage,
	waTemplates json.RawMessage,
	agentPhoneNumber *string,
) (models.Campaign, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return models.Campaign{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var campaign models.Campaign

	err = tx.QueryRow(
		ctx,
		`
		INSERT INTO campaigns (
			name,
			system_prompt,
			voice_config,
			whatsapp_templates,
			agent_phone_number
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5
		)
		RETURNING
			id,
			name,
			system_prompt,
			voice_config,
			whatsapp_templates,
			default_resume_asset_id,
			default_diagram_asset_id,
			agent_phone_number,
			active,
			version,
			archived_at,
			created_at,
			updated_at
		`,
		name,
		systemPrompt,
		voiceConfig,
		waTemplates,
		agentPhoneNumber,
	).Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.SystemPrompt,
		&campaign.VoiceConfig,
		&campaign.WhatsappTemplates,
		&campaign.DefaultResumeAssetID,
		&campaign.DefaultDiagramAssetID,
		&campaign.AgentPhoneNumber,
		&campaign.Active,
		&campaign.Version,
		&campaign.ArchivedAt,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	)
	if err != nil {
		return models.Campaign{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO campaign_versions (
			campaign_id,
			version,
			system_prompt,
			voice_config,
			whatsapp_templates,
			default_resume_asset_id,
			default_diagram_asset_id,
			agent_phone_number
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8
		)
		`,
		campaign.ID,
		campaign.Version,
		campaign.SystemPrompt,
		campaign.VoiceConfig,
		campaign.WhatsappTemplates,
		campaign.DefaultResumeAssetID,
		campaign.DefaultDiagramAssetID,
		campaign.AgentPhoneNumber,
	)
	if err != nil {
		return models.Campaign{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Campaign{}, err
	}

	return campaign, nil
}

func (r *CampaignRepo) Get(
	ctx context.Context,
	id uuid.UUID,
) (models.Campaign, error) {
	var campaign models.Campaign

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			name,
			system_prompt,
			voice_config,
			whatsapp_templates,
			default_resume_asset_id,
			default_diagram_asset_id,
			agent_phone_number,
			active,
			version,
			archived_at,
			created_at,
			updated_at
		FROM campaigns
		WHERE id = $1
		`,
		id,
	).Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.SystemPrompt,
		&campaign.VoiceConfig,
		&campaign.WhatsappTemplates,
		&campaign.DefaultResumeAssetID,
		&campaign.DefaultDiagramAssetID,
		&campaign.AgentPhoneNumber,
		&campaign.Active,
		&campaign.Version,
		&campaign.ArchivedAt,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	)

	return campaign, err
}

func (r *CampaignRepo) GetActive(
	ctx context.Context,
	id uuid.UUID,
) (models.Campaign, error) {
	var campaign models.Campaign

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			name,
			system_prompt,
			voice_config,
			whatsapp_templates,
			default_resume_asset_id,
			default_diagram_asset_id,
			agent_phone_number,
			active,
			version,
			archived_at,
			created_at,
			updated_at
		FROM campaigns
		WHERE id = $1
		  AND active = true
		  AND archived_at IS NULL
		`,
		id,
	).Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.SystemPrompt,
		&campaign.VoiceConfig,
		&campaign.WhatsappTemplates,
		&campaign.DefaultResumeAssetID,
		&campaign.DefaultDiagramAssetID,
		&campaign.AgentPhoneNumber,
		&campaign.Active,
		&campaign.Version,
		&campaign.ArchivedAt,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	)

	return campaign, err
}

func (r *CampaignRepo) DefaultCampaignID(
	ctx context.Context,
) (uuid.UUID, error) {
	var id uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			(s.value #>> '{}')::uuid
		FROM system_settings s
		JOIN campaigns c
			ON c.id = (s.value #>> '{}')::uuid
		WHERE s.key = 'default_campaign_id'
		  AND c.active = true
		  AND c.archived_at IS NULL
		`,
	).Scan(&id)

	return id, err
}
func (r *CampaignRepo) SetAssets(
	ctx context.Context,
	id uuid.UUID,
	resumeAssetID *uuid.UUID,
	diagramAssetID *uuid.UUID,
) (models.Campaign, error) {
	var campaign models.Campaign

	err := r.db.Pool.QueryRow(
		ctx,
		`
		UPDATE campaigns
		SET
			default_resume_asset_id = $2,
			default_diagram_asset_id = $3,
			updated_at = now()
		WHERE id = $1
		  AND archived_at IS NULL
		RETURNING
			id,
			name,
			system_prompt,
			voice_config,
			whatsapp_templates,
			default_resume_asset_id,
			default_diagram_asset_id,
			agent_phone_number,
			active,
			version,
			archived_at,
			created_at,
			updated_at
		`,
		id,
		resumeAssetID,
		diagramAssetID,
	).Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.SystemPrompt,
		&campaign.VoiceConfig,
		&campaign.WhatsappTemplates,
		&campaign.DefaultResumeAssetID,
		&campaign.DefaultDiagramAssetID,
		&campaign.AgentPhoneNumber,
		&campaign.Active,
		&campaign.Version,
		&campaign.ArchivedAt,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	)

	return campaign, err
}

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/repository"
)

type CampaignHandler struct {
	campaigns *repository.CampaignRepo
}

func NewCampaignHandler(
	campaigns *repository.CampaignRepo,
) *CampaignHandler {
	return &CampaignHandler{
		campaigns: campaigns,
	}
}

func (h *CampaignHandler) List(
	c *gin.Context,
) {
	campaigns, err := h.campaigns.List(
		c.Request.Context(),
	)
	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"campaigns": campaigns,
		},
	)
}

type createCampaignRequest struct {
	Name              string          `json:"name" binding:"required"`
	SystemPrompt      *string         `json:"system_prompt"`
	VoiceConfig       json.RawMessage `json:"voice_config"`
	WhatsappTemplates json.RawMessage `json:"whatsapp_templates"`
	AgentPhoneNumber  *string         `json:"agent_phone_number"`
}

func (h *CampaignHandler) Create(
	c *gin.Context,
) {
	var req createCampaignRequest

	if err := c.ShouldBindJSON(
		&req,
	); err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	req.Name = strings.TrimSpace(
		req.Name,
	)

	if req.Name == "" {
		jsonError(
			c,
			http.StatusBadRequest,
			"name is required",
		)
		return
	}

	if req.SystemPrompt != nil {
		value := strings.TrimSpace(
			*req.SystemPrompt,
		)

		if value == "" {
			req.SystemPrompt = nil
		} else {
			req.SystemPrompt = &value
		}
	}

	if req.AgentPhoneNumber != nil {
		value := strings.TrimSpace(
			*req.AgentPhoneNumber,
		)

		if value == "" {
			req.AgentPhoneNumber = nil
		} else {
			req.AgentPhoneNumber = &value
		}
	}

	voiceConfig := req.VoiceConfig

	if len(voiceConfig) == 0 {
		voiceConfig = json.RawMessage(`{}`)
	}

	waTemplates := req.WhatsappTemplates

	if len(waTemplates) == 0 {
		waTemplates = json.RawMessage(`{}`)
	}

	campaign, err := h.campaigns.Create(
		c.Request.Context(),
		req.Name,
		req.SystemPrompt,
		voiceConfig,
		waTemplates,
		req.AgentPhoneNumber,
	)
	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.JSON(
		http.StatusCreated,
		campaign,
	)
}

func (h *CampaignHandler) Get(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	campaign, err := h.campaigns.Get(
		c.Request.Context(),
		id,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		jsonError(
			c,
			http.StatusNotFound,
			"campaign not found",
		)
		return
	}

	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.JSON(
		http.StatusOK,
		campaign,
	)
}

type updateCampaignAssetsRequest struct {
	ResumeAssetID  *uuid.UUID `json:"resume_asset_id"`
	DiagramAssetID *uuid.UUID `json:"diagram_asset_id"`
}

func (h *CampaignHandler) UpdateAssets(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	var req updateCampaignAssetsRequest

	if err := c.ShouldBindJSON(
		&req,
	); err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	campaign, err := h.campaigns.SetAssets(
		c.Request.Context(),
		id,
		req.ResumeAssetID,
		req.DiagramAssetID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonError(
			c,
			http.StatusNotFound,
			"campaign not found",
		)
		return
	}

	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.JSON(
		http.StatusOK,
		campaign,
	)
}

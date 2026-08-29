package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type CallbackHandler struct {
	callbacks *repository.CallbackRepo
	calls     *repository.CallRepo
}

func NewCallbackHandler(
	callbacks *repository.CallbackRepo,
	calls *repository.CallRepo,
) *CallbackHandler {
	return &CallbackHandler{
		callbacks: callbacks,
		calls:     calls,
	}
}

func (h *CallbackHandler) List(
	c *gin.Context,
) {
	callbacks, err := h.callbacks.List(
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
			"callbacks": callbacks,
		},
	)
}

type createCallbackRequest struct {
	LeadID       string `json:"lead_id" binding:"required"`
	ScheduledFor string `json:"scheduled_for" binding:"required"`
	Timezone     string `json:"timezone"`
}

func (h *CallbackHandler) Create(
	c *gin.Context,
) {
	var req createCallbackRequest

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

	leadID, err := uuid.Parse(
		strings.TrimSpace(
			req.LeadID,
		),
	)
	if err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			"invalid lead_id",
		)
		return
	}

	scheduledFor, err := time.Parse(
		time.RFC3339,
		strings.TrimSpace(
			req.ScheduledFor,
		),
	)
	if err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			"invalid scheduled_for",
		)
		return
	}

	if !scheduledFor.After(
		time.Now(),
	) {
		jsonError(
			c,
			http.StatusBadRequest,
			"scheduled_for must be in the future",
		)
		return
	}

	timezone := strings.TrimSpace(
		req.Timezone,
	)

	if timezone == "" {
		timezone = "Asia/Kolkata"
	}

	if _, err := time.LoadLocation(
		timezone,
	); err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			"invalid timezone",
		)
		return
	}

	campaignID, err :=
		h.calls.ActiveCampaignID(
			c.Request.Context(),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		jsonError(
			c,
			http.StatusBadRequest,
			"no active campaign available",
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

	call, err := h.calls.Create(
		c.Request.Context(),
		leadID,
		campaignID,
	)
	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		jsonError(
			c,
			http.StatusBadRequest,
			"lead not found or campaign is not active",
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

	confidence := 1.0

	resolvedFrom, err := json.Marshal(
		map[string]any{
			"resolution_source":  "manual",
			"confidence":         confidence,
			"needs_confirmation": false,
			"timezone":           timezone,
		},
	)
	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	requestedTimeText := scheduledFor.
		In(
			time.FixedZone(
				timezone,
				0,
			),
		).
		Format(
			"02 Jan 2006 03:04 PM",
		)

	callback, err :=
		h.callbacks.Create(
			c.Request.Context(),
			repository.CreateCallbackInput{
				CallID:               call.ID,
				LeadID:               leadID,
				RequestedTimeText:    requestedTimeText,
				ScheduledFor:         &scheduledFor,
				Timezone:             timezone,
				ResolutionConfidence: &confidence,
				ResolutionSource:     "manual",
				ResolvedFrom:         resolvedFrom,
				Status:               models.CallbackScheduled,
			},
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
		gin.H{
			"callback": callback,
		},
	)
}

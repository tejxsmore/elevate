package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/repository"
	"elevate/internal/service"
)

type CallHandler struct {
	callService     *service.CallService
	calls           *repository.CallRepo
	discovery       *repository.DiscoveryRepo
	classifications *repository.ClassificationRepo
	recordings      *service.RecordingService
}

func NewCallHandler(
	callService *service.CallService,
	calls *repository.CallRepo,
	discovery *repository.DiscoveryRepo,
	classifications *repository.ClassificationRepo,
	recordings *service.RecordingService,
) *CallHandler {
	return &CallHandler{
		callService:     callService,
		calls:           calls,
		discovery:       discovery,
		classifications: classifications,
		recordings:      recordings,
	}
}

type triggerCallRequest struct {
	PhoneE164         string     `json:"phone_e164" binding:"required"`
	Name              *string    `json:"name"`
	PreferredLanguage *string    `json:"preferred_language"`
	CampaignID        *uuid.UUID `json:"campaign_id"`
}

func (h *CallHandler) Trigger(
	c *gin.Context,
) {
	var req triggerCallRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	call, err := h.callService.TriggerCall(
		c.Request.Context(),
		service.TriggerCallInput{
			PhoneE164:         req.PhoneE164,
			Name:              req.Name,
			PreferredLanguage: req.PreferredLanguage,
			CampaignID:        req.CampaignID,
		},
	)
	if err != nil {
		if errors.Is(
			err,
			service.ErrInvalidPhoneE164,
		) {
			jsonError(
				c,
				http.StatusBadRequest,
				err.Error(),
			)
			return
		}

		jsonError(
			c,
			http.StatusBadGateway,
			err.Error(),
		)
		return
	}

	c.JSON(
		http.StatusAccepted,
		gin.H{
			"call":    call,
			"message": "queued for dialing",
		},
	)
}

func (h *CallHandler) List(
	c *gin.Context,
) {
	limit, offset := parseLimitOffset(c)

	calls, err := h.calls.List(
		c.Request.Context(),
		limit,
		offset,
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
			"calls":  calls,
			"limit":  limit,
			"offset": offset,
		},
	)
}

func (h *CallHandler) Get(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	call, err := h.calls.Get(
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
			"call not found",
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
		call,
	)
}

func (h *CallHandler) Recording(
	c *gin.Context,
) {
	if h.recordings == nil {
		jsonError(
			c,
			http.StatusServiceUnavailable,
			"recording service is not configured",
		)
		return
	}

	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	call, err := h.calls.Get(
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
			"call not found",
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

	if call.RecordingSID == nil ||
		strings.TrimSpace(
			*call.RecordingSID,
		) == "" {
		jsonError(
			c,
			http.StatusNotFound,
			"recording not available",
		)
		return
	}

	key := "recordings/" +
		call.ID.String() +
		"/" +
		strings.TrimSpace(
			*call.RecordingSID,
		) +
		".mp3"

	url, err := h.recordings.SignedURL(
		c.Request.Context(),
		key,
		15*time.Minute,
	)
	if err != nil {
		jsonError(
			c,
			http.StatusBadGateway,
			err.Error(),
		)
		return
	}

	c.Header(
		"Cache-Control",
		"no-store",
	)

	c.Redirect(
		http.StatusTemporaryRedirect,
		url,
	)
}

func (h *CallHandler) Transcript(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	segments, err := h.calls.Transcript(
		c.Request.Context(),
		id,
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
			"segments": segments,
		},
	)
}

func (h *CallHandler) Actions(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	actions, err := h.calls.Actions(
		c.Request.Context(),
		id,
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
			"actions": actions,
		},
	)
}

func (h *CallHandler) Discovery(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	discovery, err := h.discovery.GetByCallID(
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
			"discovery profile not found",
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
		discovery,
	)
}

func (h *CallHandler) Classification(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	classification, err := h.classifications.Latest(
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
			"classification not found",
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
		classification,
	)
}

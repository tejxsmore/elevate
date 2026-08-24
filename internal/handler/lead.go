package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type LeadHandler struct {
	leads *repository.LeadRepo
}

func NewLeadHandler(leads *repository.LeadRepo) *LeadHandler {
	return &LeadHandler{leads: leads}
}

func (h *LeadHandler) List(c *gin.Context) {
	limit, offset := parseLimitOffset(c)
	leads, err := h.leads.List(c.Request.Context(), limit, offset)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"leads": leads, "limit": limit, "offset": offset})
}

type createLeadRequest struct {
	PhoneE164         string  `json:"phone_e164" binding:"required"`
	Name              *string `json:"name"`
	PreferredLanguage *string `json:"preferred_language"`
	Source            *string `json:"source"`
}

func (h *LeadHandler) Create(c *gin.Context) {
	var req createLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}

	lang := models.LanguageUnknown
	if req.PreferredLanguage != nil {
		lang = models.LanguageCode(*req.PreferredLanguage)
	}

	l, err := h.leads.Upsert(c.Request.Context(), req.PhoneE164, req.Name, lang, req.Source)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, l)
}

func (h *LeadHandler) Get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	l, err := h.leads.Get(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonError(c, http.StatusNotFound, "lead not found")
		return
	}
	if err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, l)
}

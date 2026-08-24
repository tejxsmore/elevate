package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"elevate/internal/repository"
	"elevate/internal/service"
)

type RecordingHandler struct {
	recordings *service.RecordingService
	calls      *repository.CallRepo
	authToken  string
}

func NewRecordingHandler(
	recordings *service.RecordingService,
	calls *repository.CallRepo,
	authToken string,
) *RecordingHandler {
	return &RecordingHandler{
		recordings: recordings,
		calls:      calls,
		authToken:  authToken,
	}
}

func (h *RecordingHandler) TwilioRecording(
	c *gin.Context,
) {
	if h.recordings == nil ||
		h.calls == nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			"recording handler is not configured",
		)
		return
	}

	if !validateTwilioSignature(
		c.Request,
		h.authToken,
	) {
		c.AbortWithStatusJSON(
			http.StatusForbidden,
			gin.H{
				"error": "invalid Twilio signature",
			},
		)
		return
	}

	callIDText := strings.TrimSpace(
		c.Query("call_id"),
	)

	if callIDText == "" {
		jsonError(
			c,
			http.StatusBadRequest,
			"missing call_id",
		)
		return
	}

	callID, err := uuid.Parse(
		callIDText,
	)
	if err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			"invalid call_id",
		)
		return
	}

	recordingSID := strings.TrimSpace(
		c.PostForm("RecordingSid"),
	)

	recordingURL := strings.TrimSpace(
		c.PostForm("RecordingUrl"),
	)

	recordingStatus := strings.ToLower(
		strings.TrimSpace(
			c.PostForm("RecordingStatus"),
		),
	)

	if recordingSID == "" {
		jsonError(
			c,
			http.StatusBadRequest,
			"missing RecordingSid",
		)
		return
	}

	if recordingStatus != "" &&
		recordingStatus != "completed" {
		c.Status(
			http.StatusNoContent,
		)
		return
	}

	if recordingURL == "" {
		jsonError(
			c,
			http.StatusBadRequest,
			"missing RecordingUrl",
		)
		return
	}

	if !h.recordings.Enabled() {
		log.Printf(
			"twilio_recording: storage disabled call_id=%s sid=%s",
			callID,
			recordingSID,
		)

		c.Status(
			http.StatusNoContent,
		)
		return
	}

	storedURL, err :=
		h.recordings.StoreTwilioRecording(
			c.Request.Context(),
			callID,
			recordingSID,
			recordingURL,
		)
	if err != nil {
		log.Printf(
			"twilio_recording: store failed call_id=%s sid=%s url=%s: %v",
			callID,
			recordingSID,
			recordingURL,
			err,
		)

		jsonError(
			c,
			http.StatusBadGateway,
			err.Error(),
		)
		return
	}

	if err := h.calls.UpdateRecording(
		c.Request.Context(),
		callID,
		recordingSID,
		storedURL,
	); err != nil {
		log.Printf(
			"twilio_recording: database update failed call_id=%s sid=%s: %v",
			callID,
			recordingSID,
			err,
		)

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.Status(
		http.StatusNoContent,
	)
}

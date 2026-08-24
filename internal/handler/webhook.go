package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"elevate/internal/service"
)

type WebhookHandler struct {
	callService     *service.CallService
	whatsappService *service.WhatsappService
	webhookService  *service.WebhookService
	authToken       string
}

func NewWebhookHandler(
	callService *service.CallService,
	whatsappService *service.WhatsappService,
	webhookService *service.WebhookService,
	authToken string,
) *WebhookHandler {
	return &WebhookHandler{
		callService:     callService,
		whatsappService: whatsappService,
		webhookService:  webhookService,
		authToken:       authToken,
	}
}

func (h *WebhookHandler) TwilioVoice(
	c *gin.Context,
) {
	callID := strings.TrimSpace(
		c.Query("call_id"),
	)

	callSID := strings.TrimSpace(
		c.PostForm("CallSid"),
	)

	payload := formPayload(c)

	relatedCallID := parseOptionalUUID(
		callID,
	)

	ingest, err := h.webhookService.Ingest(
		c.Request.Context(),
		"twilio",
		"voice",
		callSID,
		payload,
		relatedCallID,
	)
	if err != nil {
		log.Printf(
			"twilio_voice: ingest failed call_id=%s call_sid=%s: %v",
			callID,
			callSID,
			err,
		)

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	twiml, err := h.callService.HandleVoiceWebhook(
		c.Request.Context(),
		callID,
		callSID,
	)
	if err != nil {
		if ingest.ShouldRun {
			_ = h.webhookService.Failed(
				c.Request.Context(),
				ingest.EventID,
				err,
			)
		}

		log.Printf(
			"twilio_voice: call_id=%s call_sid=%s: %v",
			callID,
			callSID,
			err,
		)

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	if ingest.ShouldRun {
		if err := h.webhookService.Processed(
			c.Request.Context(),
			ingest.EventID,
		); err != nil {
			log.Printf(
				"twilio_voice: mark processed call_id=%s event=%s: %v",
				callID,
				ingest.EventID,
				err,
			)

			jsonError(
				c,
				http.StatusInternalServerError,
				err.Error(),
			)
			return
		}
	}

	c.Data(
		http.StatusOK,
		"text/xml; charset=utf-8",
		[]byte(twiml),
	)
}

func (h *WebhookHandler) TwilioStatus(
	c *gin.Context,
) {
	callID := strings.TrimSpace(
		c.Query("call_id"),
	)

	if callID == "" {
		callID = strings.TrimSpace(
			c.PostForm("CallID"),
		)
	}

	callSID := strings.TrimSpace(
		c.PostForm("CallSid"),
	)

	callStatus := strings.TrimSpace(
		c.PostForm("CallStatus"),
	)

	durationText := strings.TrimSpace(
		c.PostForm("CallDuration"),
	)

	var duration *int

	if durationText != "" {
		if value, err := strconv.Atoi(
			durationText,
		); err == nil {
			duration = &value
		}
	}

	payload := formPayload(c)

	relatedCallID := parseOptionalUUID(
		callID,
	)

	ingest, err := h.webhookService.Ingest(
		c.Request.Context(),
		"twilio",
		"call_status",
		callSID+"|"+callStatus,
		payload,
		relatedCallID,
	)
	if err != nil {
		log.Printf(
			"twilio_status: ingest failed call_id=%s call_sid=%s status=%s: %v",
			callID,
			callSID,
			callStatus,
			err,
		)

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	if !ingest.ShouldRun {
		c.Status(
			http.StatusNoContent,
		)
		return
	}

	err = h.callService.HandleStatusWebhook(
		c.Request.Context(),
		callID,
		callSID,
		callStatus,
		duration,
	)
	if err != nil {
		_ = h.webhookService.Failed(
			c.Request.Context(),
			ingest.EventID,
			err,
		)

		log.Printf(
			"twilio_status: call_id=%s call_sid=%s status=%s duration=%s: %v",
			callID,
			callSID,
			callStatus,
			durationText,
			err,
		)

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	if err := h.webhookService.Processed(
		c.Request.Context(),
		ingest.EventID,
	); err != nil {
		log.Printf(
			"twilio_status: mark processed call_id=%s event=%s: %v",
			callID,
			ingest.EventID,
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

func (h *WebhookHandler) TwilioWhatsapp(
	c *gin.Context,
) {
	jsonError(
		c,
		http.StatusNotImplemented,
		"inbound whatsapp handling not yet wired",
	)
}

func (h *WebhookHandler) TwilioWhatsappStatus(
	c *gin.Context,
) {
	messageSID := strings.TrimSpace(
		firstNonEmpty(
			c.PostForm("MessageSid"),
			c.PostForm("SmsSid"),
		),
	)

	messageStatus := strings.TrimSpace(
		firstNonEmpty(
			c.PostForm("MessageStatus"),
			c.PostForm("SmsStatus"),
		),
	)

	errorCode := strings.TrimSpace(
		c.PostForm("ErrorCode"),
	)

	errorMessage := strings.TrimSpace(
		firstNonEmpty(
			c.PostForm("ErrorMessage"),
			c.PostForm("ChannelStatusMessage"),
		),
	)

	eventType := strings.TrimSpace(
		c.PostForm("EventType"),
	)

	if messageSID == "" {
		jsonError(
			c,
			http.StatusBadRequest,
			"missing MessageSid",
		)
		return
	}

	if messageStatus == "" {
		jsonError(
			c,
			http.StatusBadRequest,
			"missing MessageStatus",
		)
		return
	}

	if errorMessage == "" &&
		errorCode != "" {
		errorMessage = "Twilio error code: " + errorCode
	}

	payload := formPayload(c)

	ingest, err := h.webhookService.Ingest(
		c.Request.Context(),
		"twilio",
		"whatsapp_status",
		messageSID+"|"+messageStatus,
		payload,
		nil,
	)
	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	if !ingest.ShouldRun {
		c.Status(
			http.StatusNoContent,
		)
		return
	}

	if err := h.whatsappService.HandleStatus(
		c.Request.Context(),
		messageSID,
		messageStatus,
		errorMessage,
		eventType,
	); err != nil {
		_ = h.webhookService.Failed(
			c.Request.Context(),
			ingest.EventID,
			err,
		)

		log.Printf(
			"twilio_whatsapp_status: sid=%s status=%s: %v",
			messageSID,
			messageStatus,
			err,
		)

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	if err := h.webhookService.Processed(
		c.Request.Context(),
		ingest.EventID,
	); err != nil {
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

func formPayload(
	c *gin.Context,
) map[string]any {
	payload := make(
		map[string]any,
	)

	if err := c.Request.ParseForm(); err != nil {
		return payload
	}

	for key, values := range c.Request.PostForm {
		if len(values) == 1 {
			payload[key] = values[0]
			continue
		}

		payload[key] = values
	}

	if value := c.Query("call_id"); value != "" {
		payload["call_id"] = value
	}

	return payload
}

func parseOptionalUUID(
	value string,
) *uuid.UUID {
	value = strings.TrimSpace(value)

	if value == "" {
		return nil
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}

	return &id
}

func firstNonEmpty(
	values ...string,
) string {
	for _, value := range values {
		value = strings.TrimSpace(value)

		if value != "" {
			return value
		}
	}

	return ""
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"elevate/internal/config"
)

type TwilioClient struct {
	AccountSID                string
	AuthToken                 string
	WhatsAppNumber            string
	WhatsAppStatusCallbackURL string
	httpClient                *http.Client
}

func NewTwilioClient(
	accountSID string,
	authToken string,
	whatsAppNumber string,
	whatsAppStatusCallbackURL ...string,
) *TwilioClient {
	statusURL := ""

	if len(whatsAppStatusCallbackURL) > 0 {
		statusURL = strings.TrimSpace(
			whatsAppStatusCallbackURL[0],
		)
	}

	return &TwilioClient{
		AccountSID: strings.TrimSpace(
			accountSID,
		),
		AuthToken: strings.TrimSpace(
			authToken,
		),
		WhatsAppNumber: strings.TrimSpace(
			whatsAppNumber,
		),
		WhatsAppStatusCallbackURL: statusURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func NewTwilioClientFromConfig(
	cfg config.TwilioConfig,
) *TwilioClient {
	return NewTwilioClient(
		cfg.AccountSID,
		cfg.AuthToken,
		cfg.WhatsAppNumber,
		cfg.WhatsAppStatusCallbackURL,
	)
}

type TwilioCallParams struct {
	To                            string
	From                          string
	VoiceURL                      string
	StatusCallbackURL             string
	StatusCallbackEvents          []string
	RecordingStatusCallbackURL    string
	RecordingStatusCallbackEvents []string
}

type twilioCallResponse struct {
	SID      string `json:"sid"`
	Code     int    `json:"code"`
	Message  string `json:"message"`
	MoreInfo string `json:"more_info"`
}

type twilioMessageResponse struct {
	SID          string      `json:"sid"`
	Status       json.Number `json:"status"`
	Code         int         `json:"code"`
	Message      string      `json:"message"`
	MoreInfo     string      `json:"more_info"`
	ErrorCode    *int        `json:"error_code"`
	ErrorMessage string      `json:"error_message"`
}

func (c *TwilioClient) PlaceCall(
	ctx context.Context,
	p TwilioCallParams,
) (string, error) {
	if c == nil {
		return "", fmt.Errorf(
			"twilio: client is nil",
		)
	}

	if strings.TrimSpace(c.AccountSID) == "" ||
		strings.TrimSpace(c.AuthToken) == "" {
		return "", fmt.Errorf(
			"twilio: missing credentials",
		)
	}

	to := strings.TrimSpace(p.To)
	from := strings.TrimSpace(p.From)
	voiceURL := strings.TrimSpace(p.VoiceURL)

	if to == "" {
		return "", fmt.Errorf(
			"twilio: missing destination number",
		)
	}

	if from == "" {
		return "", fmt.Errorf(
			"twilio: missing source number",
		)
	}

	if voiceURL == "" {
		return "", fmt.Errorf(
			"twilio: missing voice URL",
		)
	}

	endpoint := fmt.Sprintf(
		"https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json",
		c.AccountSID,
	)

	form := url.Values{}

	form.Set("To", to)
	form.Set("From", from)
	form.Set("Url", voiceURL)

	if statusURL := strings.TrimSpace(
		p.StatusCallbackURL,
	); statusURL != "" {
		form.Set("StatusCallback", statusURL)
		form.Set("StatusCallbackMethod", "POST")

		events := append(
			[]string(nil),
			p.StatusCallbackEvents...,
		)

		if len(events) == 0 {
			events = []string{
				"initiated",
				"ringing",
				"answered",
				"completed",
			}
		}

		for _, event := range events {
			event = strings.TrimSpace(event)

			if event != "" {
				form.Add(
					"StatusCallbackEvent",
					event,
				)
			}
		}
	}

	if recordingURL := strings.TrimSpace(
		p.RecordingStatusCallbackURL,
	); recordingURL != "" {
		form.Set("Record", "true")
		form.Set("RecordingStatusCallback", recordingURL)
		form.Set("RecordingStatusCallbackMethod", "POST")

		events := append(
			[]string(nil),
			p.RecordingStatusCallbackEvents...,
		)

		if len(events) == 0 {
			events = []string{
				"completed",
			}
		}

		for _, event := range events {
			event = strings.TrimSpace(event)

			if event != "" {
				form.Add(
					"RecordingStatusCallbackEvent",
					event,
				)
			}
		}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf(
			"twilio: build call request: %w",
			err,
		)
	}

	req.SetBasicAuth(
		c.AccountSID,
		c.AuthToken,
	)

	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf(
			"twilio: call request failed: %w",
			err,
		)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf(
			"twilio: read call response: %w",
			err,
		)
	}

	var out twilioCallResponse

	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf(
			"twilio: decode call response: %w",
			err,
		)
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf(
			"twilio: %d %s (code %d, more info: %s)",
			resp.StatusCode,
			strings.TrimSpace(out.Message),
			out.Code,
			strings.TrimSpace(out.MoreInfo),
		)
	}

	out.SID = strings.TrimSpace(out.SID)

	if out.SID == "" {
		return "", fmt.Errorf(
			"twilio: call response contained no SID",
		)
	}

	return out.SID, nil
}

func normalizeWhatsAppNumber(
	number string,
) string {
	number = strings.TrimSpace(number)

	if strings.HasPrefix(number, "whatsapp:") {
		return number
	}

	return "whatsapp:" + number
}

func (c *TwilioClient) SendWhatsApp(
	ctx context.Context,
	to string,
	body string,
	mediaURLs []string,
) (string, error) {
	if c == nil {
		return "", fmt.Errorf(
			"twilio: client is nil",
		)
	}

	if strings.TrimSpace(c.AccountSID) == "" ||
		strings.TrimSpace(c.AuthToken) == "" {
		return "", fmt.Errorf(
			"twilio: missing credentials",
		)
	}

	if strings.TrimSpace(c.WhatsAppNumber) == "" {
		return "", fmt.Errorf(
			"twilio: WhatsApp number is not configured",
		)
	}

	if strings.TrimSpace(to) == "" {
		return "", fmt.Errorf(
			"twilio: WhatsApp recipient is empty",
		)
	}

	if strings.TrimSpace(body) == "" &&
		len(mediaURLs) == 0 {
		return "", fmt.Errorf(
			"twilio: WhatsApp message has no body or media",
		)
	}

	endpoint := fmt.Sprintf(
		"https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json",
		c.AccountSID,
	)

	form := url.Values{}

	form.Set(
		"To",
		normalizeWhatsAppNumber(to),
	)

	form.Set(
		"From",
		normalizeWhatsAppNumber(
			c.WhatsAppNumber,
		),
	)

	if strings.TrimSpace(body) != "" {
		form.Set("Body", body)
	}

	seenMedia := make(
		map[string]struct{},
	)

	for _, mediaURL := range mediaURLs {
		mediaURL = strings.TrimSpace(mediaURL)

		if mediaURL == "" {
			continue
		}

		if _, exists := seenMedia[mediaURL]; exists {
			continue
		}

		seenMedia[mediaURL] = struct{}{}

		form.Add(
			"MediaUrl",
			mediaURL,
		)
	}

	if strings.TrimSpace(
		c.WhatsAppStatusCallbackURL,
	) != "" {
		form.Set(
			"StatusCallback",
			strings.TrimSpace(
				c.WhatsAppStatusCallbackURL,
			),
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf(
			"twilio: build WhatsApp request: %w",
			err,
		)
	}

	req.SetBasicAuth(
		c.AccountSID,
		c.AuthToken,
	)

	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf(
			"twilio: WhatsApp request failed: %w",
			err,
		)
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf(
			"twilio: read WhatsApp response: %w",
			err,
		)
	}

	var out twilioMessageResponse

	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return "", fmt.Errorf(
			"twilio: decode WhatsApp response: %w",
			err,
		)
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		if strings.TrimSpace(out.ErrorMessage) != "" {
			return "", fmt.Errorf(
				"twilio: WhatsApp %d: %s",
				resp.StatusCode,
				out.ErrorMessage,
			)
		}

		return "", fmt.Errorf(
			"twilio: WhatsApp %d %s (code %d, more info: %s)",
			resp.StatusCode,
			strings.TrimSpace(out.Message),
			out.Code,
			strings.TrimSpace(out.MoreInfo),
		)
	}

	out.SID = strings.TrimSpace(out.SID)

	if out.SID == "" {
		return "", fmt.Errorf(
			"twilio: WhatsApp response contained no SID",
		)
	}

	return out.SID, nil
}

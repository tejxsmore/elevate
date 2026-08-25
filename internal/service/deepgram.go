package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"elevate/internal/config"
)

type DeepgramAudioFormat struct {
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sample_rate"`
	Container  string `json:"container,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
}

type DeepgramAudioSettings struct {
	Input  DeepgramAudioFormat `json:"input"`
	Output DeepgramAudioFormat `json:"output"`
}

type DeepgramProvider struct {
	Type          string   `json:"type"`
	Model         string   `json:"model,omitempty"`
	ModelID       string   `json:"model_id,omitempty"`
	Version       string   `json:"version,omitempty"`
	Temperature   float64  `json:"temperature,omitempty"`
	ReasoningMode string   `json:"reasoning_mode,omitempty"`
	Language      string   `json:"language,omitempty"`
	LanguageHints []string `json:"language_hints,omitempty"`
	Voice         any      `json:"voice,omitempty"`
	Speed         float64  `json:"speed,omitempty"`
	SmartFormat   bool     `json:"smart_format,omitempty"`
}

type DeepgramEndpoint struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

type DeepgramListenConfig struct {
	Provider DeepgramProvider `json:"provider"`
}

type DeepgramFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type DeepgramThinkConfig struct {
	Provider  DeepgramProvider   `json:"provider"`
	Prompt    string             `json:"prompt,omitempty"`
	Functions []DeepgramFunction `json:"functions,omitempty"`
}

type DeepgramSpeakConfig struct {
	Provider DeepgramProvider  `json:"provider"`
	Endpoint *DeepgramEndpoint `json:"endpoint,omitempty"`
}

type DeepgramAgentConfig struct {
	Listen   DeepgramListenConfig `json:"listen"`
	Think    DeepgramThinkConfig  `json:"think"`
	Speak    DeepgramSpeakConfig  `json:"speak"`
	Greeting string               `json:"greeting,omitempty"`
}

type DeepgramFlags struct {
	History bool `json:"history"`
}

type DeepgramSettingsMessage struct {
	Type  string                `json:"type"`
	Audio DeepgramAudioSettings `json:"audio"`
	Agent DeepgramAgentConfig   `json:"agent"`
	Flags *DeepgramFlags        `json:"flags,omitempty"`
}

type AgentEventType string

const (
	EventWelcome              AgentEventType = "Welcome"
	EventSettingsApplied      AgentEventType = "SettingsApplied"
	EventConversationText     AgentEventType = "ConversationText"
	EventUserStartedSpeaking  AgentEventType = "UserStartedSpeaking"
	EventAgentThinking        AgentEventType = "AgentThinking"
	EventAgentStartedSpeaking AgentEventType = "AgentStartedSpeaking"
	EventAgentAudioDone       AgentEventType = "AgentAudioDone"
	EventFunctionCallRequest  AgentEventType = "FunctionCallRequest"
	EventFunctionCallResponse AgentEventType = "FunctionCallResponse"
	EventInjectionRefused     AgentEventType = "InjectionRefused"
	EventWarning              AgentEventType = "Warning"
	EventError                AgentEventType = "Error"
	EventAudioChunk           AgentEventType = "AudioChunk"
	EventHistory              AgentEventType = "History"
	EventListenUpdated        AgentEventType = "ListenUpdated"
	EventSpeakUpdated         AgentEventType = "SpeakUpdated"
	EventThinkUpdated         AgentEventType = "ThinkUpdated"
	EventPromptUpdated        AgentEventType = "PromptUpdated"
)

type AgentFunctionCall struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ClientSide       bool   `json:"client_side"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type AgentEvent struct {
	Type       AgentEventType
	Raw        json.RawMessage
	Audio      []byte
	Role       string
	Content    string
	ReceivedAt time.Time
	Functions  []AgentFunctionCall
	Languages  []string
}

type conversationTextPayload struct {
	Type            string   `json:"type"`
	Role            string   `json:"role"`
	Content         string   `json:"content"`
	Languages       []string `json:"languages,omitempty"`
	LanguagesHinted []string `json:"languages_hinted,omitempty"`
}

type functionCallRequestPayload struct {
	Type      string              `json:"type"`
	Functions []AgentFunctionCall `json:"functions"`
}

type DeepgramAgentClient struct {
	cfg config.DeepgramConfig
}

func NewDeepgramAgentClient(
	cfg config.DeepgramConfig,
) *DeepgramAgentClient {
	return &DeepgramAgentClient{
		cfg: cfg,
	}
}

type AgentConn struct {
	ws        *websocket.Conn
	events    chan AgentEvent
	errCh     chan error
	closeOnce sync.Once
	writeMu   sync.Mutex
}

func (c *DeepgramAgentClient) Connect(
	ctx context.Context,
	settings DeepgramSettingsMessage,
) (*AgentConn, error) {
	if c == nil {
		return nil, fmt.Errorf(
			"deepgram: client is nil",
		)
	}

	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return nil, fmt.Errorf(
			"deepgram: API key is missing",
		)
	}

	if strings.TrimSpace(c.cfg.AgentURL) == "" {
		return nil, fmt.Errorf(
			"deepgram: agent URL is missing",
		)
	}

	dialer := websocket.DefaultDialer

	headers := http.Header{}

	headers.Set(
		"Authorization",
		"Token "+strings.TrimSpace(
			c.cfg.APIKey,
		),
	)

	ws, _, err := dialer.DialContext(
		ctx,
		c.cfg.AgentURL,
		headers,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"deepgram: websocket connect: %w",
			err,
		)
	}

	conn := &AgentConn{
		ws:     ws,
		events: make(chan AgentEvent, 256),
		errCh:  make(chan error, 1),
	}

	if err := conn.writeJSON(settings); err != nil {
		_ = ws.Close()

		return nil, fmt.Errorf(
			"deepgram: send settings: %w",
			err,
		)
	}

	go conn.readLoop()

	return conn, nil
}

func (a *AgentConn) Events() <-chan AgentEvent {
	return a.events
}

func (a *AgentConn) Errors() <-chan error {
	return a.errCh
}

func (a *AgentConn) writeJSON(
	value any,
) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	return a.ws.WriteJSON(value)
}

func (a *AgentConn) SendAudio(
	audio []byte,
) error {
	if len(audio) == 0 {
		return nil
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	return a.ws.WriteMessage(
		websocket.BinaryMessage,
		audio,
	)
}

func (a *AgentConn) SendFunctionCallResponse(
	id string,
	name string,
	content string,
) error {
	return a.writeJSON(
		map[string]any{
			"type":    "FunctionCallResponse",
			"id":      id,
			"name":    name,
			"content": content,
		},
	)
}

func (a *AgentConn) UpdateListen(
	provider DeepgramProvider,
) error {
	return a.writeJSON(
		map[string]any{
			"type": "UpdateListen",
			"listen": map[string]any{
				"provider": provider,
			},
		},
	)
}

func (a *AgentConn) UpdateSpeak(
	provider DeepgramProvider,
) error {
	return a.writeJSON(
		map[string]any{
			"type": "UpdateSpeak",
			"speak": map[string]any{
				"provider": provider,
			},
		},
	)
}

func (a *AgentConn) UpdatePrompt(
	prompt string,
) error {
	return a.writeJSON(
		map[string]string{
			"type":   "UpdatePrompt",
			"prompt": prompt,
		},
	)
}

func (a *AgentConn) InjectAgentMessage(
	content string,
	behavior string,
) error {
	message := map[string]string{
		"type":    "InjectAgentMessage",
		"content": content,
	}

	if strings.TrimSpace(behavior) != "" {
		message["behavior"] = behavior
	}

	return a.writeJSON(message)
}

func (a *AgentConn) KeepAlive() error {
	return a.writeJSON(
		map[string]string{
			"type": "KeepAlive",
		},
	)
}

func (a *AgentConn) Close() error {
	var err error

	a.closeOnce.Do(func() {
		err = a.ws.Close()
	})

	return err
}

func (a *AgentConn) emit(
	event AgentEvent,
) bool {
	select {
	case a.events <- event:
		return true
	default:
		return false
	}
}

func (a *AgentConn) emitError(
	err error,
) {
	if err == nil {
		return
	}

	select {
	case a.errCh <- err:
	default:
	}
}

func (a *AgentConn) readLoop() {
	defer close(a.events)
	defer close(a.errCh)

	for {
		messageType, data, err :=
			a.ws.ReadMessage()

		if err != nil {
			a.emitError(err)
			return
		}

		now := time.Now()

		if messageType == websocket.BinaryMessage {
			a.emit(
				AgentEvent{
					Type: EventAudioChunk,
					Audio: append(
						[]byte(nil),
						data...,
					),
					ReceivedAt: now,
				},
			)

			continue
		}

		if messageType != websocket.TextMessage {
			continue
		}

		var head struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(
			data,
			&head,
		); err != nil {
			continue
		}

		event := AgentEvent{
			Type: AgentEventType(
				head.Type,
			),
			Raw: append(
				json.RawMessage(nil),
				data...,
			),
			ReceivedAt: now,
		}

		switch AgentEventType(head.Type) {
		case EventConversationText:
			var payload conversationTextPayload

			if err := json.Unmarshal(
				data,
				&payload,
			); err == nil {
				event.Role = payload.Role
				event.Content = payload.Content

				event.Languages = append(
					[]string(nil),
					payload.Languages...,
				)

				if len(event.Languages) == 0 &&
					len(payload.LanguagesHinted) > 0 {
					event.Languages = append(
						[]string(nil),
						payload.LanguagesHinted...,
					)
				}
			}

		case EventFunctionCallRequest:
			var payload functionCallRequestPayload

			if err := json.Unmarshal(
				data,
				&payload,
			); err == nil {
				event.Functions = append(
					[]AgentFunctionCall(nil),
					payload.Functions...,
				)
			}
		}

		a.emit(event)
	}
}

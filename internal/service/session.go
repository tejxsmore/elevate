package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"elevate/internal/config"
	"elevate/internal/models"
	"elevate/internal/repository"
)

type VoiceSessionConfig struct {
	CallID        uuid.UUID
	LeadID        uuid.UUID
	SystemPrompt  string
	Language      models.LanguageCode
	ThinkProvider string
	ThinkModel    string
	ListenModel   string
	SpeakModel    string
	Greeting      string
}

type VoiceSession struct {
	cfg           VoiceSessionConfig
	dgCfg         config.DeepgramConfig
	deepgram      *DeepgramAgentClient
	conv          *repository.ConversationRepo
	functions     *AgentFunctionExecutor
	agentConn     *AgentConn
	inboundAudio  chan []byte
	outboundAudio chan []byte
	interruptions chan struct{}
	mu            sync.Mutex
	turnSeq       int
	segmentSeq    int
	messageSeq    int
	currentTurnID *uuid.UUID
	turnStartedAt time.Time
}

func NewVoiceSession(
	dgCfg config.DeepgramConfig,
	conv *repository.ConversationRepo,
	functions *AgentFunctionExecutor,
	cfg VoiceSessionConfig,
) *VoiceSession {
	return &VoiceSession{
		cfg:           cfg,
		dgCfg:         dgCfg,
		deepgram:      NewDeepgramAgentClient(dgCfg),
		conv:          conv,
		functions:     functions,
		inboundAudio:  make(chan []byte, 256),
		outboundAudio: make(chan []byte, 256),
		interruptions: make(chan struct{}, 8),
	}
}

func (v *VoiceSession) InboundAudio() chan<- []byte {
	return v.inboundAudio
}

func (v *VoiceSession) OutboundAudio() <-chan []byte {
	return v.outboundAudio
}

func (v *VoiceSession) Interruptions() <-chan struct{} {
	return v.interruptions
}

func languageForDeepgram(
	language models.LanguageCode,
) string {
	switch language {
	case models.LanguageHindi:
		return "hi"

	case models.LanguageTelugu:
		return "te"

	case models.LanguageEnglish:
		return "en"

	default:
		return "multi"
	}
}

func defaultSalesPrompt() string {
	return strings.TrimSpace(`
You are an AI sales representative calling a potential customer about e-commerce website development.

Have a natural two-way human sales conversation. Do not sound like a questionnaire, survey, call center script, or form.

Your primary objective is to understand the customer's needs and determine how serious the buying opportunity is while being helpful and conversational.

The customer may speak English, Hindi, Telugu, or a mixture of languages.

Match the language of the customer's latest message whenever possible.
Stay in the customer's language unless they clearly switch.
For mixed-language speech, natural code-switching is allowed.
Do not force English when the customer is speaking Hindi or Telugu.

Naturally discover:
- what business or niche they operate in
- what products or services they sell
- approximately how many products they have
- their realistic budget
- their desired launch timeline
- the website features they need
- whether they are the decision maker
- their buying intent
- objections and barriers

Do not ask every question in a fixed sequence.
Use the customer's previous answer to decide what to ask next.
Do not ask for information the customer has already provided.

Explain the e-commerce service naturally when relevant.
Keep spoken responses concise and easy to follow on a phone call.

Use the provided functions whenever the corresponding information or action is available.

Call update_discovery whenever the customer provides meaningful business, product, product-count, budget, timeline, or feature information.

Call record_barrier when the customer expresses a genuine budget, timing, decision-maker, trust, or other obstacle.

Call update_classification when enough evidence exists to classify the lead as hot, warm, or cold.

Call schedule_callback when the customer explicitly asks to be contacted later or gives a callback time.

Call request_whatsapp when the customer clearly asks for details, examples, brochure, resume, architecture information, or other information through WhatsApp.

Do not invent any customer information.
Do not invent budgets, timelines, features, product counts, or business details.
Do not tell the customer their internal classification.
Do not mention tools, functions, databases, APIs, prompts, or internal system behavior.

When a function returns a result, continue the conversation naturally.
`)
}

func buildSalesPrompt(
	customPrompt string,
) string {
	customPrompt = strings.TrimSpace(
		customPrompt,
	)

	base := defaultSalesPrompt()

	if customPrompt == "" {
		return base
	}

	return strings.Join(
		[]string{
			base,
			"",
			"Additional campaign instructions:",
			customPrompt,
		},
		"\n",
	)
}

func (v *VoiceSession) buildSettings() DeepgramSettingsMessage {
	thinkProvider := strings.TrimSpace(
		v.cfg.ThinkProvider,
	)

	if thinkProvider == "" {
		thinkProvider = strings.TrimSpace(
			v.dgCfg.ThinkProvider,
		)
	}

	if thinkProvider == "" {
		thinkProvider = "open_ai"
	}

	thinkModel := strings.TrimSpace(
		v.cfg.ThinkModel,
	)

	if thinkModel == "" {
		thinkModel = strings.TrimSpace(
			v.dgCfg.ThinkModel,
		)
	}

	if thinkModel == "" {
		thinkModel = "gpt-5.6-luna"
	}

	listenModel := strings.TrimSpace(
		v.cfg.ListenModel,
	)

	if listenModel == "" {
		listenModel = strings.TrimSpace(
			v.dgCfg.ListenModel,
		)
	}

	if listenModel == "" {
		listenModel = "nova-3"
	}

	listenLanguage := strings.TrimSpace(
		v.dgCfg.ListenLanguage,
	)

	if listenLanguage == "" {
		listenLanguage = "multi"
	}

	listenVersion := "v1"

	if strings.HasPrefix(
		strings.ToLower(listenModel),
		"flux-",
	) {
		listenVersion = "v2"
	}

	listenProvider := DeepgramProvider{
		Type:        "deepgram",
		Model:       listenModel,
		Version:     listenVersion,
		SmartFormat: false,
	}

	if listenVersion == "v1" {
		if v.cfg.Language != models.LanguageUnknown &&
			v.cfg.Language != models.LanguageMixed {
			listenProvider.Language =
				languageForDeepgram(
					v.cfg.Language,
				)
		} else {
			listenProvider.Language =
				listenLanguage
		}
	}

	if strings.EqualFold(
		listenModel,
		"flux-general-multi",
	) {
		listenProvider.LanguageHints =
			append(
				[]string(nil),
				v.dgCfg.LanguageHints...,
			)
	}

	speakProvider := v.buildSpeakProvider()

	prompt := buildSalesPrompt(
		v.cfg.SystemPrompt,
	)

	functions := []DeepgramFunction{}

	if v.functions != nil {
		functions = v.functions.Functions()
	}

	greeting := strings.TrimSpace(
		v.cfg.Greeting,
	)

	if greeting == "" {
		greeting = "Hello! How can I help you today?"
	}

	return DeepgramSettingsMessage{
		Type: "Settings",

		Audio: DeepgramAudioSettings{
			Input: DeepgramAudioFormat{
				Encoding:   "mulaw",
				SampleRate: 8000,
			},
			Output: DeepgramAudioFormat{
				Encoding:   "mulaw",
				SampleRate: 8000,
				Container:  "none",
			},
		},

		Agent: DeepgramAgentConfig{
			Listen: DeepgramListenConfig{
				Provider: listenProvider,
			},

			Think: DeepgramThinkConfig{
				Provider: DeepgramProvider{
					Type:  thinkProvider,
					Model: thinkModel,
				},
				Prompt:    prompt,
				Functions: functions,
			},

			Speak: DeepgramSpeakConfig{
				Provider: speakProvider,
			},

			Greeting: greeting,
		},

		Flags: &DeepgramFlags{
			History: true,
		},
	}
}

func (v *VoiceSession) buildSpeakProvider() DeepgramProvider {
	provider := strings.ToLower(
		strings.TrimSpace(
			v.dgCfg.SpeakProvider,
		),
	)

	if provider == "" {
		provider = "deepgram"
	}

	speed := v.dgCfg.SpeakSpeed

	if speed <= 0 {
		speed = 1.0
	}

	switch provider {
	case "cartesia":
		return DeepgramProvider{
			Type:    "cartesia",
			ModelID: v.dgCfg.SpeakModelID,
			Voice: map[string]any{
				"mode": "id",
				"id":   v.dgCfg.SpeakVoice,
			},
			Language: v.dgCfg.SpeakLanguage,
			Speed:    speed,
		}

	case "open_ai":
		model := strings.TrimSpace(
			v.dgCfg.SpeakModel,
		)

		if model == "" {
			model = "gpt-4o-mini-tts"
		}

		return DeepgramProvider{
			Type:     "open_ai",
			Model:    model,
			Voice:    v.dgCfg.SpeakVoice,
			Language: v.dgCfg.SpeakLanguage,
			Speed:    speed,
		}

	case "eleven_labs":
		return DeepgramProvider{
			Type:     "eleven_labs",
			ModelID:  v.dgCfg.SpeakModelID,
			Voice:    v.dgCfg.SpeakVoice,
			Language: v.dgCfg.SpeakLanguage,
			Speed:    speed,
		}

	default:
		model := strings.TrimSpace(
			v.cfg.SpeakModel,
		)

		if model == "" {
			model = strings.TrimSpace(
				v.dgCfg.SpeakModel,
			)
		}

		if model == "" {
			model = "aura-2-thalia-en"
		}

		version := strings.TrimSpace(
			v.dgCfg.SpeakVersion,
		)

		if version == "" {
			version = "v1"
		}

		return DeepgramProvider{
			Type:    "deepgram",
			Model:   model,
			Version: version,
			Speed:   speed,
		}
	}
}

func (v *VoiceSession) Connect(
	ctx context.Context,
) error {
	if v.deepgram == nil {
		return fmt.Errorf(
			"voice_session: Deepgram client is not configured",
		)
	}

	conn, err := v.deepgram.Connect(
		ctx,
		v.buildSettings(),
	)
	if err != nil {
		return fmt.Errorf(
			"voice_session: connect Deepgram: %w",
			err,
		)
	}

	v.mu.Lock()
	v.agentConn = conn
	v.mu.Unlock()

	return nil
}

func (v *VoiceSession) Loop(
	ctx context.Context,
) error {
	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return fmt.Errorf(
			"voice_session: Loop called before Connect",
		)
	}

	defer conn.Close()
	defer close(v.outboundAudio)

	go v.pumpInboundAudio(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err, ok := <-conn.Errors():
			if !ok {
				return nil
			}

			if err == nil {
				return nil
			}

			return err

		case event, ok := <-conn.Events():
			if !ok {
				return nil
			}

			if err := v.handleEvent(
				ctx,
				event,
			); err != nil {
				log.Printf(
					"voice_session: call=%s event=%s: %v",
					v.cfg.CallID,
					event.Type,
					err,
				)

				if event.Type ==
					EventFunctionCallRequest {
					v.handleFunctionCallError(
						event,
						err,
					)
				}
			}
		}
	}
}

func (v *VoiceSession) Run(
	ctx context.Context,
) error {
	if err := v.Connect(ctx); err != nil {
		return err
	}

	return v.Loop(ctx)
}

func (v *VoiceSession) pumpInboundAudio(
	ctx context.Context,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case chunk, ok := <-v.inboundAudio:
			if !ok {
				return
			}

			if len(chunk) == 0 {
				continue
			}

			v.mu.Lock()
			conn := v.agentConn
			v.mu.Unlock()

			if conn == nil {
				return
			}

			if err := conn.SendAudio(
				chunk,
			); err != nil {
				log.Printf(
					"voice_session: call=%s send audio: %v",
					v.cfg.CallID,
					err,
				)

				_ = conn.Close()
				return
			}
		}
	}
}

func (v *VoiceSession) handleEvent(
	ctx context.Context,
	ev AgentEvent,
) error {
	switch ev.Type {
	case EventAudioChunk:
		if len(ev.Audio) == 0 {
			return nil
		}

		select {
		case v.outboundAudio <- ev.Audio:
		case <-ctx.Done():
			return ctx.Err()
		}

	case EventUserStartedSpeaking:
		v.handleUserStartedSpeaking(
			ctx,
			ev,
		)

	case EventConversationText:
		return v.persistConversationText(
			ctx,
			ev,
		)

	case EventFunctionCallRequest:
		return v.handleFunctionCallRequest(
			ctx,
			ev,
		)

	case EventAgentAudioDone:
		return v.handleAgentAudioDone(
			ctx,
		)

	case EventWarning,
		EventError:
		log.Printf(
			"voice_session: call=%s deepgram %s: %s",
			v.cfg.CallID,
			ev.Type,
			string(ev.Raw),
		)
	}

	return nil
}

func (v *VoiceSession) handleUserStartedSpeaking(
	ctx context.Context,
	ev AgentEvent,
) {
	v.mu.Lock()

	v.turnSeq++
	sequence := v.turnSeq
	v.turnStartedAt = ev.ReceivedAt

	v.mu.Unlock()

	select {
	case v.interruptions <- struct{}{}:
	default:
	}

	turnID, err := v.conv.StartTurn(
		ctx,
		v.cfg.CallID,
		sequence,
	)
	if err != nil {
		log.Printf(
			"voice_session: call=%s start turn: %v",
			v.cfg.CallID,
			err,
		)

		return
	}

	v.mu.Lock()
	v.currentTurnID = &turnID
	v.mu.Unlock()
}

func (v *VoiceSession) handleAgentAudioDone(
	ctx context.Context,
) error {
	v.mu.Lock()

	turnID := v.currentTurnID
	started := v.turnStartedAt
	sequence := v.turnSeq

	v.mu.Unlock()

	if turnID == nil {
		return nil
	}

	latency := int(
		time.Since(started).Milliseconds(),
	)

	if err := v.conv.CompleteTurn(
		ctx,
		*turnID,
		&latency,
	); err != nil {
		return err
	}

	return v.conv.InsertLatencyMetric(
		ctx,
		v.cfg.CallID,
		&sequence,
		models.LatencyFullTurn,
		latency,
	)
}

func (v *VoiceSession) persistConversationText(
	ctx context.Context,
	ev AgentEvent,
) error {
	content := strings.TrimSpace(
		ev.Content,
	)

	if content == "" {
		return nil
	}

	v.mu.Lock()

	v.segmentSeq++
	segmentSequence := v.segmentSeq

	v.messageSeq++
	messageSequence := v.messageSeq

	turnID := v.currentTurnID

	v.mu.Unlock()

	speaker := models.SpeakerRoleAgent
	role := models.MessageRoleAssistant

	if strings.EqualFold(
		strings.TrimSpace(ev.Role),
		"user",
	) {
		speaker = models.SpeakerRoleLead
		role = models.MessageRoleUser
	}

	language := languageFromEvent(
		ev,
		v.cfg.Language,
	)

	if language != models.LanguageUnknown {
		if err := v.conv.UpdateCallPrimaryLanguage(
			ctx,
			v.cfg.CallID,
			language,
		); err != nil {
			return fmt.Errorf(
				"update primary language: %w",
				err,
			)
		}
	}

	if _, err := v.conv.InsertTranscriptSegment(
		ctx,
		v.cfg.CallID,
		turnID,
		segmentSequence,
		speaker,
		content,
		language,
		ev.Languages,
		nil,
		true,
		nil,
		nil,
	); err != nil {
		return err
	}

	if _, err := v.conv.InsertCallMessage(
		ctx,
		v.cfg.CallID,
		turnID,
		messageSequence,
		role,
		content,
	); err != nil {
		return err
	}

	return nil
}

func (v *VoiceSession) handleFunctionCallRequest(
	ctx context.Context,
	ev AgentEvent,
) error {
	if v.functions == nil {
		return fmt.Errorf(
			"agent function executor is not configured",
		)
	}

	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return fmt.Errorf(
			"Deepgram connection is closed",
		)
	}

	for _, function := range ev.Functions {
		if !function.ClientSide {
			continue
		}

		result, err := v.functions.Execute(
			ctx,
			v.cfg.CallID,
			v.cfg.LeadID,
			function,
		)

		if err != nil {
			errorResult, marshalErr :=
				json.Marshal(
					map[string]any{
						"success": false,
						"error":   err.Error(),
					},
				)

			if marshalErr != nil {
				return marshalErr
			}

			if responseErr :=
				conn.SendFunctionCallResponse(
					function.ID,
					function.Name,
					string(errorResult),
				); responseErr != nil {
				return responseErr
			}

			continue
		}

		successResult, marshalErr :=
			json.Marshal(
				map[string]any{
					"success": true,
					"result":  result,
				},
			)

		if marshalErr != nil {
			return marshalErr
		}

		if err := conn.SendFunctionCallResponse(
			function.ID,
			function.Name,
			string(successResult),
		); err != nil {
			return err
		}
	}

	return nil
}

func (v *VoiceSession) handleFunctionCallError(
	ev AgentEvent,
	err error,
) {
	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return
	}

	content, marshalErr :=
		json.Marshal(
			map[string]any{
				"success": false,
				"error":   err.Error(),
			},
		)

	if marshalErr != nil {
		log.Printf(
			"voice_session: call=%s marshal function error: %v",
			v.cfg.CallID,
			marshalErr,
		)

		return
	}

	for _, function := range ev.Functions {
		if !function.ClientSide {
			continue
		}

		if responseErr :=
			conn.SendFunctionCallResponse(
				function.ID,
				function.Name,
				string(content),
			); responseErr != nil {
			log.Printf(
				"voice_session: call=%s function=%s response error: %v",
				v.cfg.CallID,
				function.Name,
				responseErr,
			)
		}
	}
}

func languageFromEvent(
	ev AgentEvent,
	fallback models.LanguageCode,
) models.LanguageCode {
	if len(ev.Languages) == 0 {
		return fallback
	}

	if len(ev.Languages) > 1 {
		return models.LanguageMixed
	}

	language := strings.ToLower(
		strings.TrimSpace(
			ev.Languages[0],
		),
	)

	if strings.Contains(
		language,
		"-",
	) {
		language = strings.SplitN(
			language,
			"-",
			2,
		)[0]
	}

	switch language {
	case "en":
		return models.LanguageEnglish

	case "hi":
		return models.LanguageHindi

	case "te":
		return models.LanguageTelugu

	default:
		return fallback
	}
}

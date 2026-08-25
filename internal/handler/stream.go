package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"elevate/internal/repository"
	"elevate/internal/service"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type MediaStreamHandler struct {
	sessions *service.VoiceSessionManager
	conv     *repository.ConversationRepo
}

func NewMediaStreamHandler(
	sessions *service.VoiceSessionManager,
	conv *repository.ConversationRepo,
) *MediaStreamHandler {
	return &MediaStreamHandler{
		sessions: sessions,
		conv:     conv,
	}
}

type twilioStreamEvent struct {
	Event string             `json:"event"`
	Start *twilioStreamStart `json:"start,omitempty"`
	Media *twilioStreamMedia `json:"media,omitempty"`
	Stop  *twilioStreamStop  `json:"stop,omitempty"`
}

type twilioStreamStart struct {
	StreamSid        string            `json:"streamSid"`
	CallSid          string            `json:"callSid"`
	CustomParameters map[string]string `json:"customParameters"`
}

type twilioStreamMedia struct {
	Track   string `json:"track"`
	Payload string `json:"payload"`
}

type twilioStreamStop struct {
	StreamSid string `json:"streamSid"`
	CallSid   string `json:"callSid"`
}

type twilioOutboundMedia struct {
	Event     string                  `json:"event"`
	StreamSid string                  `json:"streamSid"`
	Media     twilioOutboundMediaBody `json:"media"`
}

type twilioOutboundMediaBody struct {
	Payload string `json:"payload"`
}

type twilioClearMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"streamSid"`
}

func (h *MediaStreamHandler) TwilioMediaStream(
	c *gin.Context,
) {
	conn, err := wsUpgrader.Upgrade(
		c.Writer,
		c.Request,
		nil,
	)
	if err != nil {
		log.Printf(
			"media_stream: upgrade failed: %v",
			err,
		)
		return
	}

	defer conn.Close()

	ctx := contextOrBackground(
		c.Request.Context(),
	)

	var (
		streamSid string
		callSID   string
		callID    uuid.UUID
		session   *service.VoiceSession
	)

	done := make(chan struct{})

	clearOutbound := make(
		chan struct{},
		16,
	)

	var stopOnce sync.Once
	var writeMu sync.Mutex

	writeJSON := func(
		value any,
	) error {
		writeMu.Lock()
		defer writeMu.Unlock()

		return conn.WriteJSON(value)
	}

	stop := func() {
		stopOnce.Do(func() {
			close(done)

			if callID == uuid.Nil {
				return
			}

			if err := h.conv.MarkCallEnded(
				context.Background(),
				callID,
			); err != nil {
				log.Printf(
					"media_stream: call=%s mark ended: %v",
					callID,
					err,
				)
			}

			h.sessions.Stop(
				callID,
			)
		})
	}

	defer stop()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var event twilioStreamEvent

		if err := json.Unmarshal(
			raw,
			&event,
		); err != nil {
			continue
		}

		switch event.Event {
		case "start":
			if event.Start == nil {
				continue
			}

			streamSid = event.Start.StreamSid
			callSID = event.Start.CallSid

			callIDText :=
				event.Start.CustomParameters["callId"]

			parsedCallID, err :=
				uuid.Parse(callIDText)

			if err != nil {
				log.Printf(
					"media_stream: invalid callId=%q: %v",
					callIDText,
					err,
				)
				return
			}

			callID = parsedCallID

			callContext, err :=
				h.conv.GetCallContext(
					ctx,
					callID,
				)

			if err != nil {
				log.Printf(
					"media_stream: call=%s context lookup failed: %v",
					callID,
					err,
				)
				return
			}

			systemPrompt := ""

			if callContext.SystemPrompt != nil {
				systemPrompt =
					*callContext.SystemPrompt
			}

			session, err =
				h.sessions.Start(
					ctx,
					service.VoiceSessionConfig{
						CallID:       callID,
						LeadID:       callContext.LeadID,
						SystemPrompt: systemPrompt,
						Language:     callContext.PreferredLanguage,
					},
				)

			if err != nil {
				log.Printf(
					"media_stream: call=%s start session failed: %v",
					callID,
					err,
				)
				return
			}

			log.Printf(
				"media_stream: call=%s session started stream_sid=%s call_sid=%s",
				callID,
				streamSid,
				callSID,
			)

			if err := h.conv.MarkCallInProgress(
				ctx,
				callID,
				streamSid,
				callSID,
			); err != nil {
				log.Printf(
					"media_stream: call=%s mark in progress: %v",
					callID,
					err,
				)
				return
			}

			go pumpOutboundAudio(
				conn,
				&writeMu,
				streamSid,
				session,
				service.NewPCMDownsampler(),
				clearOutbound,
				done,
			)

			go func(
				activeSession *service.VoiceSession,
				activeStreamSID string,
			) {
				for {
					select {
					case <-done:
						return

					case <-activeSession.Interruptions():
						activeSession.ClearOutboundAudio()

						select {
						case clearOutbound <- struct{}{}:
						default:
						}

						if err := writeJSON(
							twilioClearMessage{
								Event:     "clear",
								StreamSid: activeStreamSID,
							},
						); err != nil {
							log.Printf(
								"media_stream: call=%s clear failed: %v",
								callID,
								err,
							)
							return
						}
					}
				}
			}(
				session,
				streamSid,
			)

		case "media":
			if session == nil ||
				event.Media == nil ||
				event.Media.Track != "inbound" {
				continue
			}

			audio, err :=
				base64.StdEncoding.DecodeString(
					event.Media.Payload,
				)

			if err != nil {
				log.Printf(
					"media_stream: call=%s invalid audio payload: %v",
					callID,
					err,
				)
				continue
			}

			if len(audio) == 0 {
				continue
			}

			select {
			case session.InboundAudio() <- audio:

			case <-done:
				return

			case <-ctx.Done():
				return
			}

		case "stop":
			stop()
			return
		}
	}
}

func pumpOutboundAudio(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	streamSid string,
	session *service.VoiceSession,
	downsampler *service.PCMDownsampler,
	clearOutbound <-chan struct{},
	done <-chan struct{},
) {
	const (
		frameSize       = 160
		frameDuration   = 20 * time.Millisecond
		startupFrames   = 8
		startupBytes    = frameSize * startupFrames
		maxPendingBytes = frameSize * 1000
	)

	type audioState struct {
		mu      sync.Mutex
		pending []byte
		started bool
		closed  bool
	}

	state := &audioState{
		pending: make(
			[]byte,
			0,
			frameSize*200,
		),
	}

	go func() {
		for {
			select {
			case <-done:
				return

			case audio, ok := <-session.OutboundAudio():
				if !ok {
					state.mu.Lock()
					state.closed = true
					state.mu.Unlock()
					return
				}

				if len(audio) == 0 {
					continue
				}

				mulawAudio :=
					downsampler.Push(audio)

				if len(mulawAudio) == 0 {
					continue
				}

				state.mu.Lock()

				state.pending = append(
					state.pending,
					mulawAudio...,
				)

				if len(state.pending) >
					maxPendingBytes {
					overflow :=
						len(state.pending) -
							maxPendingBytes

					state.pending =
						append(
							state.pending[:0],
							state.pending[overflow:]...,
						)
				}

				state.mu.Unlock()
			}
		}
	}()

	ticker := time.NewTicker(
		frameDuration,
	)

	defer ticker.Stop()

	chunkCount := 0

	for {
		select {
		case <-done:
			log.Printf(
				"media_stream: stream=%s outbound pump stopped, chunks_sent=%d",
				streamSid,
				chunkCount,
			)
			return

		case <-clearOutbound:
			state.mu.Lock()

			state.pending =
				state.pending[:0]

			state.started = false

			state.mu.Unlock()

			downsampler.Reset()

		case <-ticker.C:
			var (
				frame       []byte
				shouldClose bool
			)

			state.mu.Lock()

			if !state.started {
				if len(state.pending) >=
					startupBytes {
					state.started = true
				} else if state.closed &&
					len(state.pending) > 0 {
					state.started = true
				}
			}

			if state.started &&
				len(state.pending) >= frameSize {
				frame = make(
					[]byte,
					frameSize,
				)

				copy(
					frame,
					state.pending[:frameSize],
				)

				copy(
					state.pending,
					state.pending[frameSize:],
				)

				state.pending =
					state.pending[:len(state.pending)-frameSize]
			}

			if state.closed &&
				len(state.pending) == 0 &&
				frame == nil {
				shouldClose = true
			}

			state.mu.Unlock()

			if shouldClose {
				log.Printf(
					"media_stream: stream=%s outbound audio drained, chunks_sent=%d",
					streamSid,
					chunkCount,
				)
				return
			}

			if len(frame) == 0 {
				continue
			}

			message :=
				twilioOutboundMedia{
					Event:     "media",
					StreamSid: streamSid,
					Media: twilioOutboundMediaBody{
						Payload: base64.StdEncoding.EncodeToString(
							frame,
						),
					},
				}

			writeMu.Lock()

			err := conn.WriteJSON(
				message,
			)

			writeMu.Unlock()

			if err != nil {
				log.Printf(
					"media_stream: stream=%s write to twilio failed: %v",
					streamSid,
					err,
				)
				return
			}

			chunkCount++
		}
	}
}

func contextOrBackground(
	ctx context.Context,
) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"

	"elevate/internal/config"
	"elevate/internal/repository"
)

type sessionHandle struct {
	cancel context.CancelFunc
}

type VoiceSessionManager struct {
	dgCfg     config.DeepgramConfig
	conv      *repository.ConversationRepo
	functions *AgentFunctionExecutor

	mu       sync.Mutex
	sessions map[uuid.UUID]*sessionHandle
}

func NewVoiceSessionManager(
	dgCfg config.DeepgramConfig,
	conv *repository.ConversationRepo,
	functions *AgentFunctionExecutor,
) *VoiceSessionManager {
	return &VoiceSessionManager{
		dgCfg:     dgCfg,
		conv:      conv,
		functions: functions,
		sessions:  make(map[uuid.UUID]*sessionHandle),
	}
}

func (m *VoiceSessionManager) Start(
	ctx context.Context,
	cfg VoiceSessionConfig,
) (*VoiceSession, error) {
	if m == nil {
		return nil, fmt.Errorf(
			"voice_session_manager: manager is nil",
		)
	}

	if cfg.CallID == uuid.Nil {
		return nil, fmt.Errorf(
			"voice_session_manager: call ID is empty",
		)
	}

	if m.conv == nil {
		return nil, fmt.Errorf(
			"voice_session_manager: conversation repository is nil",
		)
	}

	if m.functions == nil {
		return nil, fmt.Errorf(
			"voice_session_manager: function executor is nil",
		)
	}

	if strings.TrimSpace(cfg.ThinkProvider) == "" {
		cfg.ThinkProvider = m.dgCfg.ThinkProvider
	}

	if strings.TrimSpace(cfg.ThinkModel) == "" {
		cfg.ThinkModel = m.dgCfg.ThinkModel
	}

	if strings.TrimSpace(cfg.ListenModel) == "" {
		cfg.ListenModel = m.dgCfg.ListenModel
	}

	if strings.TrimSpace(cfg.SpeakModel) == "" {
		cfg.SpeakModel = m.dgCfg.SpeakModel
	}

	cfg.Greeting = salesGreeting()

	if strings.TrimSpace(
		m.dgCfg.APIKey,
	) == "" {
		return nil, fmt.Errorf(
			"voice_session_manager: Deepgram API key is missing",
		)
	}

	if strings.TrimSpace(
		m.dgCfg.AgentURL,
	) == "" {
		return nil, fmt.Errorf(
			"voice_session_manager: Deepgram agent URL is missing",
		)
	}

	sessionCtx, cancel := context.WithCancel(
		ctx,
	)

	session := NewVoiceSession(
		m.dgCfg,
		m.conv,
		m.functions,
		cfg,
	)

	if err := session.Connect(
		sessionCtx,
	); err != nil {
		cancel()

		return nil, fmt.Errorf(
			"voice_session_manager: call=%s connect: %w",
			cfg.CallID,
			err,
		)
	}

	handle := &sessionHandle{
		cancel: cancel,
	}

	m.mu.Lock()

	old := m.sessions[cfg.CallID]

	m.sessions[cfg.CallID] = handle

	m.mu.Unlock()

	if old != nil {
		old.cancel()
	}

	go func(
		callID uuid.UUID,
		handle *sessionHandle,
	) {
		err := session.Loop(
			sessionCtx,
		)

		cancel()

		if err != nil &&
			!errors.Is(
				err,
				context.Canceled,
			) &&
			!errors.Is(
				err,
				context.DeadlineExceeded,
			) {
			log.Printf(
				"voice_session_manager: call=%s: %v",
				callID,
				err,
			)
		}

		m.mu.Lock()

		current, ok := m.sessions[callID]

		if ok && current == handle {
			delete(
				m.sessions,
				callID,
			)
		}

		m.mu.Unlock()
	}(
		cfg.CallID,
		handle,
	)

	return session, nil
}

func (m *VoiceSessionManager) Stop(
	callID uuid.UUID,
) {
	if m == nil ||
		callID == uuid.Nil {
		return
	}

	m.mu.Lock()

	handle, ok := m.sessions[callID]

	if ok {
		delete(
			m.sessions,
			callID,
		)
	}

	m.mu.Unlock()

	if ok && handle != nil {
		handle.cancel()
	}
}

func (m *VoiceSessionManager) StopAll() {
	if m == nil {
		return
	}

	m.mu.Lock()

	handles := make(
		[]*sessionHandle,
		0,
		len(m.sessions),
	)

	for callID, handle := range m.sessions {
		delete(
			m.sessions,
			callID,
		)

		handles = append(
			handles,
			handle,
		)
	}

	m.mu.Unlock()

	for _, handle := range handles {
		if handle != nil {
			handle.cancel()
		}
	}
}

func (m *VoiceSessionManager) ActiveCount() int {
	if m == nil {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.sessions)
}

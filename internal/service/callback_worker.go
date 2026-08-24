package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"elevate/internal/repository"
)

type CallbackWorker struct {
	callbacks *repository.CallbackRepo
	callSvc   *CallService
	interval  time.Duration
}

func NewCallbackWorker(
	callbacks *repository.CallbackRepo,
	callSvc *CallService,
	interval time.Duration,
) *CallbackWorker {
	if interval <= 0 {
		interval = time.Second
	}

	return &CallbackWorker{
		callbacks: callbacks,
		callSvc:   callSvc,
		interval:  interval,
	}
}

func (w *CallbackWorker) Start(
	ctx context.Context,
) {
	go w.run(ctx)
}

func (w *CallbackWorker) run(
	ctx context.Context,
) {
	ticker := time.NewTicker(
		w.interval,
	)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			w.process(ctx)
		}
	}
}

func (w *CallbackWorker) process(
	ctx context.Context,
) {
	claimed, err := w.callbacks.ClaimAndCreateFollowUpCall(
		ctx,
	)
	if err != nil {
		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return
		}

		if errors.Is(
			err,
			context.Canceled,
		) || errors.Is(
			err,
			context.DeadlineExceeded,
		) {
			return
		}

		log.Printf(
			"callback_worker: claim failed: %v",
			err,
		)

		return
	}

	callbackID := claimed.Callback.ID
	call := claimed.Call

	placedCall, err := w.callSvc.PlaceExistingCall(
		ctx,
		call,
	)
	if err != nil {
		log.Printf(
			"callback_worker: callback=%s call=%s placement failed: %v",
			callbackID,
			call.ID,
			err,
		)

		if rescheduleErr := w.callbacks.RescheduleAfterFailure(
			ctx,
			callbackID,
			5*time.Minute,
		); rescheduleErr != nil {
			log.Printf(
				"callback_worker: callback=%s reschedule failed: %v",
				callbackID,
				rescheduleErr,
			)
		}

		return
	}

	if err := w.callbacks.MarkCallPlaced(
		ctx,
		callbackID,
		placedCall.ID,
	); err != nil {
		log.Printf(
			"callback_worker: callback=%s mark placed failed: %v",
			callbackID,
			err,
		)
	}
}

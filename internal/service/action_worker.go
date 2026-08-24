package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/models"
	"elevate/internal/repository"
)

const (
	defaultActionWorkerInterval = 250 * time.Millisecond
	actionLeaseRefreshInterval  = 30 * time.Second
)

type ActionWorker struct {
	repo     *repository.ActionRepo
	executor *ActionExecutor
	interval time.Duration
	workerID string
}

func NewActionWorker(
	actionRepo *repository.ActionRepo,
	executor *ActionExecutor,
	interval time.Duration,
) *ActionWorker {
	if interval <= 0 {
		interval = defaultActionWorkerInterval
	}

	return &ActionWorker{
		repo:     actionRepo,
		executor: executor,
		interval: interval,
		workerID: "action-worker-" + uuid.NewString(),
	}
}

func (w *ActionWorker) Start(
	ctx context.Context,
) {
	if w == nil {
		return
	}

	go w.run(ctx)
}

func (w *ActionWorker) run(
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

func (w *ActionWorker) process(
	ctx context.Context,
) {
	if w == nil ||
		w.repo == nil ||
		w.executor == nil {
		return
	}

	if err := w.repo.RecoverExpired(ctx); err != nil {
		log.Printf(
			"action_worker: recover expired actions: %v",
			err,
		)
	}

	action, err := w.repo.ClaimNext(
		ctx,
		w.workerID,
	)
	if err != nil {
		if errors.Is(
			err,
			context.Canceled,
		) || errors.Is(
			err,
			context.DeadlineExceeded,
		) {
			return
		}

		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return
		}

		log.Printf(
			"action_worker: claim action failed: %v",
			err,
		)

		return
	}

	start := time.Now()

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	leaseDone := make(chan struct{})

	go w.refreshLease(
		execCtx,
		action,
		leaseDone,
	)

	executeErr := w.executor.Execute(
		execCtx,
		action,
	)

	close(leaseDone)

	if executeErr != nil {
		if markErr := w.repo.MarkFailed(
			ctx,
			action.ID,
			executeErr.Error(),
		); markErr != nil &&
			!errors.Is(
				markErr,
				pgx.ErrNoRows,
			) {
			log.Printf(
				"action_worker: action=%s mark failed: %v",
				action.ID,
				markErr,
			)
		}

		return
	}

	latency := int(
		time.Since(start).Milliseconds(),
	)

	if err := w.repo.MarkCompleted(
		ctx,
		action.ID,
		&latency,
	); err != nil &&
		!errors.Is(
			err,
			pgx.ErrNoRows,
		) {
		log.Printf(
			"action_worker: action=%s mark completed: %v",
			action.ID,
			err,
		)
	}
}

func (w *ActionWorker) refreshLease(
	ctx context.Context,
	action models.CallAction,
	done <-chan struct{},
) {
	ticker := time.NewTicker(
		actionLeaseRefreshInterval,
	)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-done:
			return

		case <-ticker.C:
			if action.LockToken == nil ||
				*action.LockToken == uuid.Nil {
				log.Printf(
					"action_worker: action=%s has no lock token",
					action.ID,
				)

				return
			}

			if err := w.repo.RefreshLease(
				ctx,
				action.ID,
				w.workerID,
				action.LockToken,
			); err != nil {
				if !errors.Is(
					err,
					pgx.ErrNoRows,
				) &&
					!errors.Is(
						err,
						context.Canceled,
					) {
					log.Printf(
						"action_worker: action=%s lease refresh failed: %v",
						action.ID,
						err,
					)
				}

				return
			}
		}
	}
}

package application

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type OutboxService struct {
	outbox    OutboxRepository
	publisher EventPublisher
	logger    *slog.Logger
	batchSize int32
}

func NewOutboxService(outbox OutboxRepository, publisher EventPublisher, batchSize int32, logger *slog.Logger) (*OutboxService, error) {
	if outbox == nil || publisher == nil {
		return nil, errors.New("application: outbox repository and publisher are required")
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxService{outbox: outbox, publisher: publisher, logger: logger, batchSize: batchSize}, nil
}

func (s *OutboxService) DrainOnce(ctx context.Context) (int, error) {
	events, err := s.outbox.Pending(ctx, s.batchSize)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, event := range events {
		if err := s.publisher.Publish(ctx, event); err != nil {
			if markErr := s.outbox.MarkFailed(ctx, event.ID, err.Error()); markErr != nil {
				return published, markErr
			}
			continue
		}
		if err := s.outbox.MarkPublished(ctx, event.ID, time.Now().UTC()); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
}

func (s *OutboxService) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.DrainOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("draining messaging outbox failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

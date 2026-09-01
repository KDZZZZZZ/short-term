package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/observability"
)

// OutboxService 清空 Outbox，并发布其中找到的事件。
//
// 发布严格发生在产生事件的事务提交之后，这正是消除数据库/消息代理双写的关键。
// 交付语义为至少一次：成功发布与标记之间如果发生崩溃，事件会再次交付，
// 因此消费者必须按 event_id 去重（docs/software-design.md 第 7.4 节）。
type OutboxService struct {
	outbox    OutboxRepository
	publisher EventPublisher
	logger    *slog.Logger
	batchSize int32
}

// NewOutboxService 组装发布循环。
func NewOutboxService(outbox OutboxRepository, publisher EventPublisher, batchSize int32, logger *slog.Logger) (*OutboxService, error) {
	if outbox == nil || publisher == nil {
		return nil, errors.New("application: the outbox repository and publisher are required")
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxService{outbox: outbox, publisher: publisher, logger: logger, batchSize: batchSize}, nil
}

// DrainOnce 发布一批事件，并报告已交付的事件数量。
//
// 某个事件失败时会记录失败并继续处理这一批：一条无法交付的事件不能阻塞其后的所有事件。
func (s *OutboxService) DrainOnce(ctx context.Context) (published int, err error) {
	events, err := s.outbox.Pending(ctx, s.batchSize)
	if err != nil {
		return 0, err
	}

	for _, event := range events {
		logger := observability.LoggerWith(ctx, s.logger).With(
			slog.String("event_id", event.ID),
			slog.String("event_type", event.Type),
		)

		if err := s.publisher.Publish(ctx, event); err != nil {
			logger.Warn("publishing an outbox event failed", slog.String("error", err.Error()))
			if markErr := s.outbox.MarkFailed(ctx, event.ID, err.Error()); markErr != nil {
				return published, markErr
			}
			continue
		}

		// 标记发生在成功发布之后。如果进程在两者之间退出，事件会被重新发布，而不会丢失。
		if err := s.outbox.MarkPublished(ctx, event.ID, time.Now().UTC()); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
}

// Run 按固定间隔清空 Outbox，直到 ctx 被取消。
func (s *OutboxService) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		published, err := s.DrainOnce(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Error("draining the outbox failed", slog.String("error", err.Error()))
		}
		if published > 0 {
			s.logger.Debug("outbox batch published", slog.Int("count", published))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

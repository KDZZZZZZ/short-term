package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type memoryOutbox struct {
	mu        sync.Mutex
	events    []Event
	published map[string]bool
	attempts  map[string]int
}

func (m *memoryOutbox) Pending(_ context.Context, limit int32) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Event, 0, limit)
	for _, event := range m.events {
		if !m.published[event.ID] {
			result = append(result, event)
		}
		if len(result) == int(limit) {
			break
		}
	}
	return result, nil
}

func (m *memoryOutbox) MarkPublished(_ context.Context, eventID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published[eventID] = true
	return nil
}

func (m *memoryOutbox) MarkFailed(_ context.Context, eventID, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[eventID]++
	return nil
}

type retryPublisher struct {
	mu        sync.Mutex
	failFirst map[string]bool
	calls     map[string]int
}

func (p *retryPublisher) Publish(_ context.Context, event Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[event.ID]++
	if p.failFirst[event.ID] && p.calls[event.ID] == 1 {
		return errors.New("broker unavailable")
	}
	return nil
}

func TestOutboxRetriesFailureWithoutBlockingLaterEvents(t *testing.T) {
	repository := &memoryOutbox{
		events:    []Event{{ID: "evt_1"}, {ID: "evt_2"}},
		published: map[string]bool{}, attempts: map[string]int{},
	}
	publisher := &retryPublisher{
		failFirst: map[string]bool{"evt_1": true}, calls: map[string]int{},
	}
	service, err := NewOutboxService(
		repository, publisher, 10, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("NewOutboxService: %v", err)
	}

	published, err := service.DrainOnce(t.Context())
	if err != nil {
		t.Fatalf("first DrainOnce: %v", err)
	}
	if published != 1 || repository.published["evt_1"] || !repository.published["evt_2"] || repository.attempts["evt_1"] != 1 {
		t.Fatalf("first drain published=%d state=%+v attempts=%+v", published, repository.published, repository.attempts)
	}

	published, err = service.DrainOnce(t.Context())
	if err != nil {
		t.Fatalf("second DrainOnce: %v", err)
	}
	if published != 1 || !repository.published["evt_1"] || publisher.calls["evt_1"] != 2 {
		t.Fatalf("retry published=%d state=%+v calls=%+v", published, repository.published, publisher.calls)
	}
}

package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConversationRejectsSelfChatAndChecksParticipants(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if _, err := NewConversation("c_1", "p_1", "u_same", "u_same", now); !errors.Is(err, ErrSelfConversation) {
		t.Fatalf("NewConversation error = %v, want ErrSelfConversation", err)
	}

	conversation, err := NewConversation("c_1", "p_1", "u_buyer", "u_seller", now)
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}
	if !conversation.IsParticipant("u_buyer") || !conversation.IsParticipant("u_seller") {
		t.Fatal("both parties must be participants")
	}
	if conversation.IsParticipant("u_intruder") {
		t.Fatal("an unrelated user was treated as a participant")
	}
}

func TestMessageContentUsesCharacterRatherThanByteLength(t *testing.T) {
	conversation, err := NewConversation("c_1", "p_1", "u_buyer", "u_seller", time.Now())
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}

	if _, err := NewMessage("m_1", conversation, "u_buyer", strings.Repeat("中", 1000), time.Now()); err != nil {
		t.Fatalf("1000 Chinese characters should be accepted: %v", err)
	}
	if _, err := NewMessage("m_2", conversation, "u_buyer", strings.Repeat("中", 1001), time.Now()); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("1001-character error = %v, want ErrInvalidContent", err)
	}
	if _, err := NewMessage("m_2b", conversation, "u_buyer", "cannot\x00store", time.Now()); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("NUL content error = %v, want ErrInvalidContent", err)
	}
	if _, err := NewMessage("m_3", conversation, "u_intruder", "hello", time.Now()); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("nonparticipant error = %v, want ErrInvalidMessage", err)
	}
}

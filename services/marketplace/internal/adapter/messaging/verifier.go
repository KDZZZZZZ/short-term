// Package messaging adapts the Messaging Service fact source to Marketplace.
package messaging

import (
	"context"
	"fmt"

	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
)

// Verifier reads conversations through the owning service. Marketplace never
// reaches into Messaging storage or trusts a client-supplied conversation id.
type Verifier struct {
	client messagingv1.MessagingServiceClient
}

// NewVerifier builds a conversation fact-source adapter.
func NewVerifier(client messagingv1.MessagingServiceClient) *Verifier {
	return &Verifier{client: client}
}

var _ application.ConversationVerifier = (*Verifier)(nil)

// Get returns the minimal projection needed to validate an intent binding.
func (v *Verifier) Get(ctx context.Context, actorID, conversationID string) (application.Conversation, error) {
	resp, err := v.client.GetConversation(grpcx.WithActor(ctx, actorID), &messagingv1.GetConversationRequest{
		ActorId: actorID, ConversationId: conversationID,
	})
	if err != nil {
		if errs.CodeOf(err) == errs.CodeResourceNotFound {
			return application.Conversation{}, errs.New(errs.CodeResourceNotFound, "会话不存在")
		}
		return application.Conversation{}, fmt.Errorf("messaging: get conversation: %w", err)
	}

	conversation := resp.GetConversation()
	if conversation == nil || conversation.GetId() == "" {
		return application.Conversation{}, fmt.Errorf("messaging: empty conversation response")
	}
	return application.Conversation{
		ID:        conversation.GetId(),
		ProductID: conversation.GetProductId(),
		BuyerID:   conversation.GetBuyerId(),
		SellerID:  conversation.GetSellerId(),
	}, nil
}

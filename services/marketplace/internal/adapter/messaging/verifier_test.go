package messaging

import (
	"context"
	"testing"

	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type captureMessagingClient struct {
	messagingv1.MessagingServiceClient
	ctx context.Context
}

func (c *captureMessagingClient) GetConversation(
	ctx context.Context,
	_ *messagingv1.GetConversationRequest,
	_ ...grpc.CallOption,
) (*messagingv1.GetConversationResponse, error) {
	c.ctx = ctx
	return &messagingv1.GetConversationResponse{Conversation: &messagingv1.ConversationItem{
		Id:        "c_01ARZ3NDEKTSV4RRFFQ69G5FAV",
		ProductId: "p_01ARZ3NDEKTSV4RRFFQ69G5FAV",
		BuyerId:   "u_buyer",
		SellerId:  "u_seller",
	}}, nil
}

func TestVerifierForwardsActorAndPublicRequestID(t *testing.T) {
	t.Parallel()

	const requestID = "m6trace-regression-0001"
	client := &captureMessagingClient{}
	verifier := NewVerifier(client)
	incoming := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		grpcx.MetadataRequestID, requestID,
	))

	if _, err := verifier.Get(incoming, "u_buyer", "c_01ARZ3NDEKTSV4RRFFQ69G5FAV"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	outgoing, ok := metadata.FromOutgoingContext(client.ctx)
	if !ok {
		t.Fatal("Messaging call has no outgoing metadata")
	}
	if got := outgoing.Get(grpcx.MetadataActorID); len(got) != 1 || got[0] != "u_buyer" {
		t.Fatalf("actor metadata = %v, want [u_buyer]", got)
	}
	if got := outgoing.Get(grpcx.MetadataRequestID); len(got) != 1 || got[0] != requestID {
		t.Fatalf("request-id metadata = %v, want [%s]", got, requestID)
	}
}

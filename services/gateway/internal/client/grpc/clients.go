// Package grpc holds the Gateway's typed clients for the internal services.
//
// Every connection is built through platform/grpcx, which refuses to dial
// without a default deadline. That is how docs/software-design.md section 7.2
// ("every downstream call carries a deadline") is enforced structurally rather
// than by review.
package grpc

import (
	"errors"
	"time"

	"google.golang.org/grpc"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
)

// Targets names the dial target of each internal service.
type Targets struct {
	Account     string
	Marketplace string
	Messaging   string
	Favorite    string
}

// Clients holds one connection per internal service.
type Clients struct {
	Account     accountv1.AccountServiceClient
	Marketplace marketplacev1.MarketplaceServiceClient
	Messaging   messagingv1.MessagingServiceClient
	Favorite    favoritev1.FavoriteServiceClient

	conns []*grpc.ClientConn
}

// Dial opens every downstream connection. A failure closes whatever was
// already opened, so a partially built set never escapes.
func Dial(targets Targets, caller string, timeout time.Duration) (*Clients, error) {
	clients := &Clients{}

	account, err := clients.dial(targets.Account, caller, timeout)
	if err != nil {
		return nil, err
	}
	marketplace, err := clients.dial(targets.Marketplace, caller, timeout)
	if err != nil {
		return nil, err
	}
	messaging, err := clients.dial(targets.Messaging, caller, timeout)
	if err != nil {
		return nil, err
	}
	favorite, err := clients.dial(targets.Favorite, caller, timeout)
	if err != nil {
		return nil, err
	}

	clients.Account = accountv1.NewAccountServiceClient(account)
	clients.Marketplace = marketplacev1.NewMarketplaceServiceClient(marketplace)
	clients.Messaging = messagingv1.NewMessagingServiceClient(messaging)
	clients.Favorite = favoritev1.NewFavoriteServiceClient(favorite)
	return clients, nil
}

// Close releases every connection.
func (c *Clients) Close() error {
	var errs []error
	for _, conn := range c.conns {
		errs = append(errs, conn.Close())
	}
	return errors.Join(errs...)
}

func (c *Clients) dial(target, caller string, timeout time.Duration) (*grpc.ClientConn, error) {
	conn, err := grpcx.Dial(grpcx.ClientOptions{
		Target:         target,
		Caller:         caller,
		DefaultTimeout: timeout,
	})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	c.conns = append(c.conns, conn)
	return conn, nil
}

// Package grpc 保存 Gateway 面向内部服务的类型化客户端。
//
// 每个连接都通过 platform/grpcx 构造，该包拒绝在没有默认截止时间时拨号。
// 这就是 docs/software-design.md 第 7.2 节（“每次下游调用都携带截止时间”）
// 如何通过结构强制执行，而不是依靠人工评审。
package grpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
)

// Targets 指定每个内部服务的拨号目标。
type Targets struct {
	Account     string
	Marketplace string
	Messaging   string
	Favorite    string
}

// Clients 为每个内部服务保存一个连接。
type Clients struct {
	Account     accountv1.AccountServiceClient
	Marketplace marketplacev1.MarketplaceServiceClient
	Messaging   messagingv1.MessagingServiceClient
	Favorite    favoritev1.FavoriteServiceClient

	conns  []*grpc.ClientConn
	health []healthTarget
}

type healthTarget struct {
	name   string
	client healthv1.HealthClient
}

// Dial 打开所有下游连接。发生失败时会关闭已经打开的连接，
// 因此不完整的客户端集合不会泄漏出去。
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
	clients.health = []healthTarget{
		{name: "account", client: healthv1.NewHealthClient(account)},
		{name: "marketplace", client: healthv1.NewHealthClient(marketplace)},
		{name: "messaging", client: healthv1.NewHealthClient(messaging)},
		{name: "favorite", client: healthv1.NewHealthClient(favorite)},
	}
	return clients, nil
}

// Ready verifies every required downstream through the standard gRPC health
// protocol. Calls run concurrently so one slow dependency consumes only one
// deadline budget rather than multiplying it by the number of services.
func (c *Clients) Ready(ctx context.Context) error {
	if c == nil || len(c.health) == 0 {
		return errors.New("gateway: downstream health clients are not configured")
	}

	errsByTarget := make(chan error, len(c.health))
	for _, target := range c.health {
		target := target
		go func() {
			response, err := target.client.Check(ctx, &healthv1.HealthCheckRequest{})
			if err != nil {
				errsByTarget <- fmt.Errorf("%s: %w", target.name, err)
				return
			}
			if response.GetStatus() != healthv1.HealthCheckResponse_SERVING {
				errsByTarget <- fmt.Errorf("%s: health status %s", target.name, response.GetStatus())
				return
			}
			errsByTarget <- nil
		}()
	}

	var healthErrors []error
	for range c.health {
		if err := <-errsByTarget; err != nil {
			healthErrors = append(healthErrors, err)
		}
	}
	return errors.Join(healthErrors...)
}

// Close 释放所有连接。
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

package partner

import (
	"context"
	"fmt"
	"time"

	kitchenv1 "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/kitchen/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type apiKeyCredentials struct {
	apiKey string
}

func (c apiKeyCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"x-api-key": c.apiKey}, nil
}

func (c apiKeyCredentials) RequireTransportSecurity() bool { return false }

type client struct {
	conn    *grpc.ClientConn
	partner kitchenv1.PartnerServiceClient
}

func dial(addr, apiKey string) (*client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(apiKeyCredentials{apiKey: apiKey}),
	)
	if err != nil {
		return nil, fmt.Errorf("dial kitchen grpc: %w", err)
	}
	return &client{conn: conn, partner: kitchenv1.NewPartnerServiceClient(conn)}, nil
}

func (c *client) close() error { return c.conn.Close() }

func (c *client) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempt := 0

	for {
		attempt++
		_, err := c.partner.GetVenue(ctx, &kitchenv1.GetVenueRequest{})
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("kitchen did not become ready in %s: %w", timeout, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempt)):
		}
	}
}

func backoff(attempt int) time.Duration {
	delay := time.Duration(attempt) * 200 * time.Millisecond
	if delay > 2*time.Second {
		return 2 * time.Second
	}
	return delay
}

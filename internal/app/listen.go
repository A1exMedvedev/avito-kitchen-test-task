package app

import (
	"context"
	"net"
)

func listen(ctx context.Context, addr string) (net.Listener, error) {
	var config net.ListenConfig
	return config.Listen(ctx, "tcp", addr)
}

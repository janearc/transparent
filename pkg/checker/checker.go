package checker

import (
	"context"
	"net"
	"time"
)

// CheckNetwork attempts to connect to a reliable external host (github.com)
// to verify internet connectivity.
func CheckNetwork(ctx context.Context) bool {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", "github.com:443")
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

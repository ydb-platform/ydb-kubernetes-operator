package testutils

import (
	"context"
	"net"
)

// mockDNSResolver creates a resolver that simulates DNS lookups, returning NXDOMAIN for unknown domains.
func mockDNSResolver(mockRecords map[string]string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Create a pipe to simulate DNS resolution
			clientConn, serverConn := net.Pipe()

			go func() {
				defer serverConn.Close()

				// Read the query (simulated, so simplified)
				buf := make([]byte, 512)
				n, _ := serverConn.Read(buf)
				requestedDomain := string(buf[:n])

				// Check if the domain exists in mock records
				ip, found := mockRecords[requestedDomain]
				if !found {
					// Return NXDOMAIN simulation
					serverConn.Write([]byte("NXDOMAIN"))
					return
				}

				// Return the resolved IP
				serverConn.Write([]byte(ip))
			}()

			return clientConn, nil
		},
	}
}

package testutils

import (
	"fmt"

	"google.golang.org/grpc/resolver"
)

// MockResolverBuilder implements the resolver.Builder interface
type MockResolverBuilder struct {
	mockAddresses map[string][]string
}

func (b *MockResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &MockResolver{
		target:        target,
		cc:            cc,
		mockAddresses: b.mockAddresses,
	}
	r.start()
	return r, nil
}

func (b *MockResolverBuilder) Scheme() string {
	return "mock"
}

// MockResolver implements the resolver.Resolver interface
type MockResolver struct {
	target        resolver.Target
	cc            resolver.ClientConn
	mockAddresses map[string][]string
}

func (r *MockResolver) start() {
	// Lookup the target in the mock addresses map
	addresses, ok := r.mockAddresses[r.target.Endpoint()]
	if !ok {
		r.cc.ReportError(fmt.Errorf("unknown target: %s", r.target.Endpoint()))
		return
	}

	// Convert addresses to gRPC addresses
	var resolved []resolver.Address
	for _, addr := range addresses {
		resolved = append(resolved, resolver.Address{Addr: addr})
	}

	// Update gRPC with the resolved addresses
	r.cc.UpdateState(resolver.State{Addresses: resolved})
}

func (r *MockResolver) ResolveNow(resolver.ResolveNowOptions) {
	// No-op as the resolution is static
}

func (r *MockResolver) Close() {}

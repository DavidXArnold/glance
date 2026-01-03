package cloud

import (
	"context"
	"testing"
	"time"
)

// testProvider is a simple Provider implementation used for cache tests.
type testProvider struct {
	calls int
	md    *Metadata
	err   error
}

func (p *testProvider) NodeMetadata(ctx context.Context, id string) (*Metadata, error) {
	p.calls++
	return p.md, p.err
}

func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache(5*time.Minute, false)

	key := "provider://node-1"
	md := &Metadata{
		InstanceType:   "m5.large",
		NodeGroup:      "ng-1",
		NodePool:       "pool-1",
		FargateProfile: "fp-1",
		CapacityType:   capacityOnDemand,
	}

	cache.Set(key, md)

	got, ok := cache.Get(key)
	if !ok {
		t.Fatalf("expected cache hit for key %q", key)
	}

	if *got != *md {
		t.Fatalf("unexpected metadata from cache: got %+v, want %+v", got, md)
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	cache := NewCache(1*time.Nanosecond, false)

	key := "provider://node-2"
	md := &Metadata{InstanceType: "c5.large"}

	cache.Set(key, md)

	// Sleep long enough for TTL to expire.
	time.Sleep(2 * time.Nanosecond)

	if _, ok := cache.Get(key); ok {
		t.Fatalf("expected cache entry for %q to be expired", key)
	}
}

func TestCacheGetOrFetch_UsesProviderAndCaches(t *testing.T) {
	providerName := "test-provider"

	p := &testProvider{md: &Metadata{InstanceType: "t3.medium"}}

	// RegisterProvider stores the factory in a package-level map; use a closure
	// that always returns the same *testProvider so we can observe call counts.
	RegisterProvider(providerName, func() Provider { return p })

	cache := NewCache(5*time.Minute, false)
	key := "provider://node-3"

	ctx := context.Background()

	// First call should hit the provider.
	md1, err := cache.GetOrFetch(ctx, providerName, key)
	if err != nil {
		t.Fatalf("GetOrFetch returned error: %v", err)
	}
	if md1 == nil || md1.InstanceType != "t3.medium" {
		t.Fatalf("unexpected metadata from GetOrFetch: %+v", md1)
	}
	if p.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", p.calls)
	}

	// Second call should come from cache without calling the provider again.
	md2, err := cache.GetOrFetch(ctx, providerName, key)
	if err != nil {
		t.Fatalf("GetOrFetch (cached) returned error: %v", err)
	}
	if md2 == nil || md2.InstanceType != "t3.medium" {
		t.Fatalf("unexpected cached metadata from GetOrFetch: %+v", md2)
	}
	if p.calls != 1 {
		t.Fatalf("expected provider to be called once after cache hit, got %d", p.calls)
	}
}

func TestLookupProvider_UnknownProviderReturnsNil(t *testing.T) {
	if p := LookupProvider("non-existent-provider"); p != nil {
		t.Fatalf("expected nil provider for unknown name, got %#v", p)
	}
}

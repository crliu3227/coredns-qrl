package qrl

import (
	"testing"
	"time"
)

func TestTokenBucketAllowsBurstThenRefills(t *testing.T) {
	limiter := newLimiter(16)
	key := "192.0.2.0/24"
	now := time.Unix(0, 0)

	for i := 0; i < 2; i++ {
		allowed, err := limiter.allowAt(key, now, 10, 2)
		if err != nil {
			t.Fatalf("allowAt() returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("allowAt() rejected initial burst request %d", i+1)
		}
	}

	allowed, err := limiter.allowAt(key, now, 10, 2)
	if err != nil {
		t.Fatalf("allowAt() returned error: %v", err)
	}
	if allowed {
		t.Fatal("allowAt() accepted a request after the burst was exhausted")
	}

	allowed, err = limiter.allowAt(key, now.Add(100*time.Millisecond), 10, 2)
	if err != nil {
		t.Fatalf("allowAt() returned error after refill: %v", err)
	}
	if !allowed {
		t.Fatal("allowAt() rejected a request after one token was refilled")
	}
}

func TestTokenBucketCapsRefillAtBurst(t *testing.T) {
	limiter := newLimiter(16)
	key := "192.0.2.0/24"
	now := time.Unix(0, 0)

	for i := 0; i < 2; i++ {
		allowed, err := limiter.allowAt(key, now, 10, 2)
		if err != nil {
			t.Fatalf("allowAt() returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("allowAt() rejected initial burst request %d", i+1)
		}
	}

	longIdle := now.Add(time.Hour)
	for i := 0; i < 2; i++ {
		allowed, err := limiter.allowAt(key, longIdle, 10, 2)
		if err != nil {
			t.Fatalf("allowAt() returned error after idle period: %v", err)
		}
		if !allowed {
			t.Fatalf("allowAt() rejected capped refill request %d", i+1)
		}
	}

	allowed, err := limiter.allowAt(key, longIdle, 10, 2)
	if err != nil {
		t.Fatalf("allowAt() returned error after capped refill: %v", err)
	}
	if allowed {
		t.Fatal("allowAt() exceeded the configured burst after a long idle period")
	}
}

func TestTokenBucketRefillsForThirtySeconds(t *testing.T) {
	limiter := newLimiter(16)
	key := "192.0.2.0/24"
	now := time.Unix(0, 0)
	const (
		rate  = 2
		burst = 3
	)

	for i := 0; i < burst; i++ {
		allowed, err := limiter.allowAt(key, now, rate, burst)
		if err != nil {
			t.Fatalf("allowAt() returned error while draining bucket: %v", err)
		}
		if !allowed {
			t.Fatalf("allowAt() rejected initial burst request %d", i+1)
		}
	}

	refilledAt := now.Add(30 * time.Second)
	for i := 0; i < burst; i++ {
		allowed, err := limiter.allowAt(key, refilledAt, rate, burst)
		if err != nil {
			t.Fatalf("allowAt() returned error after 30 seconds: %v", err)
		}
		if !allowed {
			t.Fatalf("allowAt() rejected refilled burst request %d", i+1)
		}
	}

	allowed, err := limiter.allowAt(key, refilledAt, rate, burst)
	if err != nil {
		t.Fatalf("allowAt() returned error after refilled burst: %v", err)
	}
	if allowed {
		t.Fatal("allowAt() exceeded burst after 30-second refill")
	}
}

func TestSourcePrefixIPv4(t *testing.T) {
	first, ok := sourcePrefix("192.0.2.7", 24, 56)
	if !ok {
		t.Fatal("sourcePrefix() rejected a valid IPv4 address")
	}
	if first != "192.0.2.0/24" {
		t.Fatalf("sourcePrefix() = %q, want %q", first, "192.0.2.0/24")
	}

	second, ok := sourcePrefix("192.0.2.200", 24, 56)
	if !ok {
		t.Fatal("sourcePrefix() rejected a second valid IPv4 address")
	}
	if first != second {
		t.Fatalf("addresses in one /24 got different keys: %q and %q", first, second)
	}

	other, ok := sourcePrefix("192.0.3.7", 24, 56)
	if !ok {
		t.Fatal("sourcePrefix() rejected a valid IPv4 address in another prefix")
	}
	if first == other {
		t.Fatalf("addresses in different /24 prefixes got the same key: %q", first)
	}
}

func TestSourcePrefixIPv6(t *testing.T) {
	first, ok := sourcePrefix("2001:db8:1200:1::1", 24, 56)
	if !ok {
		t.Fatal("sourcePrefix() rejected a valid IPv6 address")
	}
	if first != "2001:db8:1200::/56" {
		t.Fatalf("sourcePrefix() = %q, want %q", first, "2001:db8:1200::/56")
	}

	second, ok := sourcePrefix("2001:db8:1200:00ff::2", 24, 56)
	if !ok {
		t.Fatal("sourcePrefix() rejected a second valid IPv6 address")
	}
	if first != second {
		t.Fatalf("addresses in one /56 got different keys: %q and %q", first, second)
	}

	scoped, ok := sourcePrefix("fe80::1%eth0", 24, 56)
	if !ok {
		t.Fatal("sourcePrefix() rejected a valid scoped IPv6 address")
	}
	if scoped != "fe80::/56" {
		t.Fatalf("sourcePrefix() = %q for scoped address, want %q", scoped, "fe80::/56")
	}
}

func TestSourcePrefixRejectsInvalidIP(t *testing.T) {
	if _, ok := sourcePrefix("not-an-ip", 24, 56); ok {
		t.Fatal("sourcePrefix() accepted an invalid IP address")
	}
}

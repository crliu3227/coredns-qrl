package qrl

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/crliu3227/coredns-qrl/cache"
)

const (
	defaultTableSize = 100000
	idleBucketTTL    = 5 * time.Minute
)

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

type limiter struct {
	table *cache.Cache
}

func sourcePrefix(rawIP string, ipv4PrefixLength, ipv6PrefixLength int) (string, bool) {
	if scope := strings.LastIndexByte(rawIP, '%'); scope >= 0 {
		rawIP = rawIP[:scope]
	}
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return "", false
	}

	if ipv4 := ip.To4(); ipv4 != nil {
		if ipv4PrefixLength <= 0 || ipv4PrefixLength > 32 {
			return "", false
		}
		mask := net.CIDRMask(ipv4PrefixLength, 32)
		return (&net.IPNet{IP: ipv4.Mask(mask), Mask: mask}).String(), true
	}

	if ipv6PrefixLength <= 0 || ipv6PrefixLength > 128 {
		return "", false
	}
	mask := net.CIDRMask(ipv6PrefixLength, 128)
	return (&net.IPNet{IP: ip.Mask(mask), Mask: mask}).String(), true
}

func newLimiter(maxTableSize int) *limiter {
	if maxTableSize <= 0 {
		maxTableSize = defaultTableSize
	}

	limiter := &limiter{table: cache.New(maxTableSize)}
	limiter.table.SetEvict(func(el interface{}) bool {
		bucket, ok := el.(*tokenBucket)
		return !ok || time.Since(bucket.lastSeen) >= idleBucketTTL
	})
	return limiter
}

func (l *limiter) allowAt(key string, now time.Time, rate float64, burst int) (bool, error) {
	if rate <= 0 {
		return false, errors.New("rate must be greater than zero")
	}
	if burst <= 0 {
		return false, errors.New("burst must be greater than zero")
	}

	result := l.table.UpdateAdd(key,
		func(el interface{}) interface{} {
			bucket, ok := el.(*tokenBucket)
			if !ok {
				return errors.New("unexpected token bucket type")
			}

			if elapsed := now.Sub(bucket.lastRefill).Seconds(); elapsed > 0 {
				bucket.tokens += elapsed * rate
				if limit := float64(burst); bucket.tokens > limit {
					bucket.tokens = limit
				}
				bucket.lastRefill = now
			}
			bucket.lastSeen = now

			if bucket.tokens < 1 {
				return false
			}
			bucket.tokens--
			return true
		},
		func() interface{} {
			return &tokenBucket{
				tokens:     float64(burst - 1),
				lastRefill: now,
				lastSeen:   now,
			}
		})

	if result == nil {
		return true, nil
	}
	if err, ok := result.(error); ok {
		return false, err
	}
	allowed, ok := result.(bool)
	if !ok {
		return false, errors.New("unexpected limiter result type")
	}
	return allowed, nil
}

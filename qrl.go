package qrl

import (
	"context"
	"errors"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var (
	errRequestRateLimit = errors.New("query rate exceeded the limit")
	globalLimiter       = newLimiter(defaultTableSize)
	qrlLog              = log.NewWithPlugin("qrl")
)

// QRL applies a process-wide query rate limit per source address prefix.
type QRL struct {
	Next plugin.Handler

	limiter          *limiter
	rate             float64
	burst            int
	ipv4PrefixLength int
	ipv6PrefixLength int
	now              func() time.Time
}

// Name implements the plugin.Handler interface.
func (q QRL) Name() string { return "qrl" }

// ServeDNS implements the plugin.Handler interface.
func (q QRL) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	key, ok := sourcePrefix(state.IP(), q.ipv4PrefixLength, q.ipv6PrefixLength)
	if !ok {
		qrlLog.Debugf("unable to parse source address %q; allowing request", state.IP())
		return plugin.NextOrFailure(q.Name(), q.Next, ctx, w, r)
	}

	limiter := q.limiter
	if limiter == nil {
		limiter = globalLimiter
	}
	now := time.Now
	if q.now != nil {
		now = q.now
	}
	allowed, err := limiter.allowAt(key, now(), q.rate, q.burst)
	if err != nil {
		qrlLog.Warningf("unable to update limiter for %q: %v; allowing request", key, err)
		return plugin.NextOrFailure(q.Name(), q.Next, ctx, w, r)
	}
	if !allowed {
		qrlLog.Debugf("request rate exceeded for source prefix %q", key)
		return dns.RcodeSuccess, errRequestRateLimit
	}

	return plugin.NextOrFailure(q.Name(), q.Next, ctx, w, r)
}

func (l *limiter) allow(key string, rate float64, burst int) (bool, error) {
	return l.allowAt(key, time.Now(), rate, burst)
}

package qrl

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestQRLAllowsRequestAndCallsNext(t *testing.T) {
	calls := 0
	qrl := QRL{
		Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			calls++
			response := new(dns.Msg)
			response.SetReply(r)
			return dns.RcodeSuccess, w.WriteMsg(response)
		}),
		limiter:          newLimiter(16),
		rate:             10,
		burst:            1,
		ipv4PrefixLength: 24,
		ipv6PrefixLength: 56,
	}

	w := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := qrl.ServeDNS(context.Background(), w, test.Case{Qname: "example.org", Qtype: dns.TypeA}.Msg())
	if err != nil {
		t.Fatalf("ServeDNS() returned error: %v", err)
	}
	if code != dns.RcodeSuccess {
		t.Fatalf("ServeDNS() code = %d, want %d", code, dns.RcodeSuccess)
	}
	if calls != 1 {
		t.Fatalf("next handler calls = %d, want 1", calls)
	}
	if w.Len == 0 {
		t.Fatal("allowed request did not write a response")
	}
}

func TestQRLRejectsExceededRequestWithoutWriting(t *testing.T) {
	calls := 0
	qrl := QRL{
		Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			calls++
			response := new(dns.Msg)
			response.SetReply(r)
			return dns.RcodeSuccess, w.WriteMsg(response)
		}),
		limiter:          newLimiter(16),
		rate:             10,
		burst:            1,
		ipv4PrefixLength: 24,
		ipv6PrefixLength: 56,
	}
	request := test.Case{Qname: "example.org", Qtype: dns.TypeA}.Msg()

	first := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := qrl.ServeDNS(context.Background(), first, request); err != nil {
		t.Fatalf("first ServeDNS() returned error: %v", err)
	}

	second := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := qrl.ServeDNS(context.Background(), second, request)
	if code != dns.RcodeSuccess {
		t.Fatalf("exceeded ServeDNS() code = %d, want %d", code, dns.RcodeSuccess)
	}
	if !errors.Is(err, errRequestRateLimit) {
		t.Fatalf("exceeded ServeDNS() error = %v, want %v", err, errRequestRateLimit)
	}
	if calls != 1 {
		t.Fatalf("next handler calls = %d, want 1", calls)
	}
	if second.Len != 0 {
		t.Fatalf("exceeded request wrote %d bytes", second.Len)
	}
}

func TestQRLHandlersShareLimiterAcrossServerBlocks(t *testing.T) {
	limiter := newLimiter(16)
	newQRL := func() QRL {
		return QRL{
			Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				response := new(dns.Msg)
				response.SetReply(r)
				return dns.RcodeSuccess, w.WriteMsg(response)
			}),
			limiter:          limiter,
			rate:             10,
			burst:            1,
			ipv4PrefixLength: 24,
			ipv6PrefixLength: 56,
		}
	}
	first := newQRL()
	second := newQRL()
	request := test.Case{Qname: "example.org", Qtype: dns.TypeA}.Msg()

	if _, err := first.ServeDNS(context.Background(), dnstest.NewRecorder(&test.ResponseWriter{}), request); err != nil {
		t.Fatalf("first handler returned error: %v", err)
	}
	if _, err := second.ServeDNS(context.Background(), dnstest.NewRecorder(&test.ResponseWriter{}), request); !errors.Is(err, errRequestRateLimit) {
		t.Fatalf("second handler error = %v, want %v", err, errRequestRateLimit)
	}
}

func TestQRLAllowsMalformedSourceAddress(t *testing.T) {
	calls := 0
	qrl := QRL{
		Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			calls++
			response := new(dns.Msg)
			response.SetReply(r)
			return dns.RcodeSuccess, w.WriteMsg(response)
		}),
		limiter:          newLimiter(16),
		rate:             10,
		burst:            1,
		ipv4PrefixLength: 24,
		ipv6PrefixLength: 56,
	}
	w := dnstest.NewRecorder(&test.ResponseWriter{RemoteIP: "not-an-ip"})

	if _, err := qrl.ServeDNS(context.Background(), w, test.Case{Qname: "example.org", Qtype: dns.TypeA}.Msg()); err != nil {
		t.Fatalf("ServeDNS() returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("next handler calls = %d, want 1", calls)
	}
}

func TestQRLThirtySecondRandomTrafficRate(t *testing.T) {
	const (
		duration          = 30 * time.Second
		requestsPerSecond = 10
		rate              = 5.0
		burst             = 10
		packets           = int(duration/time.Second) * requestsPerSecond
	)

	start := time.Unix(0, 0)
	random := rand.New(rand.NewSource(1))
	timestamps := make([]time.Time, 0, packets)
	for second := 0; second < int(duration/time.Second); second++ {
		for i := 0; i < requestsPerSecond; i++ {
			offset := time.Duration(random.Int63n(int64(time.Second)))
			timestamps = append(timestamps, start.Add(time.Duration(second)*time.Second+offset))
		}
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i].Before(timestamps[j]) })

	now := start
	allowed := 0
	rejected := 0
	qrl := QRL{
		Next: test.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			allowed++
			response := new(dns.Msg)
			response.SetReply(r)
			return dns.RcodeSuccess, w.WriteMsg(response)
		}),
		limiter:          newLimiter(16),
		rate:             rate,
		burst:            burst,
		ipv4PrefixLength: 24,
		ipv6PrefixLength: 56,
		now:              func() time.Time { return now },
	}
	request := test.Case{Qname: "example.org", Qtype: dns.TypeA}.Msg()

	for i, timestamp := range timestamps {
		now = timestamp
		code, err := qrl.ServeDNS(context.Background(), dnstest.NewRecorder(&test.ResponseWriter{}), request)
		if code != dns.RcodeSuccess {
			t.Fatalf("request %d returned code %d, want %d", i+1, code, dns.RcodeSuccess)
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, errRequestRateLimit) {
			t.Fatalf("request %d returned unexpected error: %v", i+1, err)
		}
		rejected++
	}

	if allowed+rejected != packets {
		t.Fatalf("processed requests = %d, want %d", allowed+rejected, packets)
	}

	elapsed := timestamps[len(timestamps)-1].Sub(timestamps[0]).Seconds()
	measuredRate := float64(allowed-burst) / elapsed
	if math.Abs(measuredRate-rate) > 0.5 {
		t.Fatalf("measured refill rate = %.3f requests/second, want %.3f", measuredRate, rate)
	}
}

func TestQRLConcurrent(t *testing.T) {
	limiter := newLimiter(16)
	now := time.Unix(0, 0)
	const (
		requests = 1000
		burst    = 100
	)

	allowed := make(chan bool, requests)
	errs := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := limiter.allowAt("192.0.2.0/24", now, 1, burst)
			allowed <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(allowed)
	close(errs)

	allowedCount := 0
	for ok := range allowed {
		if ok {
			allowedCount++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent allowAt() returned error: %v", err)
		}
	}
	if allowedCount != burst {
		t.Fatalf("concurrent allowed requests = %d, want %d", allowedCount, burst)
	}
}

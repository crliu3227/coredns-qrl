package qrl

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestParseQRL(t *testing.T) {
	c := caddy.NewTestController("dns", `qrl {
		requests-per-second 100.5
		burst 200
		ipv4-prefix-length 20
		ipv6-prefix-length 64
	}`)
	config, err := parse(c)
	if err != nil {
		t.Fatalf("parse() returned error: %v", err)
	}
	if config.rate != 100.5 {
		t.Fatalf("rate = %v, want %v", config.rate, 100.5)
	}
	if config.burst != 200 {
		t.Fatalf("burst = %d, want %d", config.burst, 200)
	}
	if config.ipv4PrefixLength != 20 {
		t.Fatalf("IPv4 prefix length = %d, want %d", config.ipv4PrefixLength, 20)
	}
	if config.ipv6PrefixLength != 64 {
		t.Fatalf("IPv6 prefix length = %d, want %d", config.ipv6PrefixLength, 64)
	}
}

func TestParseQRLDefaultsBurstAndPrefixes(t *testing.T) {
	c := caddy.NewTestController("dns", `qrl {
		requests-per-second 2.5
	}`)
	config, err := parse(c)
	if err != nil {
		t.Fatalf("parse() returned error: %v", err)
	}
	if config.burst != 3 {
		t.Fatalf("default burst = %d, want %d", config.burst, 3)
	}
	if config.ipv4PrefixLength != 24 {
		t.Fatalf("default IPv4 prefix length = %d, want %d", config.ipv4PrefixLength, 24)
	}
	if config.ipv6PrefixLength != 56 {
		t.Fatalf("default IPv6 prefix length = %d, want %d", config.ipv6PrefixLength, 56)
	}
}

func TestParseQRLRejectsInvalidConfiguration(t *testing.T) {
	tests := []string{
		`qrl`,
		`qrl {}`,
		`qrl { requests-per-second 0 }`,
		`qrl { requests-per-second -1 }`,
		`qrl { requests-per-second invalid }`,
		`qrl { requests-per-second 10 burst 0 }`,
		`qrl { requests-per-second 10 burst -1 }`,
		`qrl { requests-per-second 10 ipv4-prefix-length 0 }`,
		`qrl { requests-per-second 10 ipv4-prefix-length 33 }`,
		`qrl { requests-per-second 10 ipv6-prefix-length 0 }`,
		`qrl { requests-per-second 10 ipv6-prefix-length 129 }`,
		`qrl { requests-per-second 10 requests-per-second 20 }`,
		`qrl { requests-per-second 10 unknown 1 }`,
		`qrl example.org { requests-per-second 10 }`,
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			c := caddy.NewTestController("dns", input)
			if _, err := parse(c); err == nil {
				t.Fatalf("parse() accepted invalid configuration %q", input)
			}
		})
	}
}

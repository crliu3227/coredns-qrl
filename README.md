# qrl

## Name

*qrl* - query rate limiting for DNS requests.

## Description

The *qrl* plugin applies a process-wide Token Bucket limit to incoming DNS
requests. Each source address is grouped into a configurable IPv4 or IPv6
network prefix. All `qrl` handlers in the same CoreDNS process share the same
limiter table, so requests received by different server blocks consume the
same bucket.

When a bucket has no available token, the request is silently dropped. The
next plugin is not called and no DNS error response is written.

This plugin limits requests in one CoreDNS process. It does not coordinate
limits between separate CoreDNS processes or replicas.

## Syntax

```txt
qrl {
    requests-per-second RATE
    burst BURST
    ipv4-prefix-length LENGTH
    ipv6-prefix-length LENGTH
}
```

- `requests-per-second` is required and must be a positive number.
- `burst` is optional and must be a positive integer. It defaults to
  `ceil(requests-per-second)`.
- `ipv4-prefix-length` is optional and defaults to `24`.
- `ipv6-prefix-length` is optional and defaults to `56`.

The plugin does not accept zone arguments. Every request handled by a server
block containing `qrl` is subject to the limiter. To cover multiple server
blocks, enable `qrl` in each block and use the same limiter settings in each
one.

## Example

```corefile
.:53 {
    qrl {
        requests-per-second 100
        burst 200
        ipv4-prefix-length 24
        ipv6-prefix-length 56
    }
    forward . 8.8.8.8
}
```

The example allows an initial burst of 200 requests from each source prefix,
then refills each bucket at 100 tokens per second.

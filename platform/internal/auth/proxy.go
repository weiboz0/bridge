package auth

import (
	"net"
	"os"
	"strings"
	"sync"
)

// IsTrustedProxy reports whether `remoteAddr` (in the "host:port" form
// http.Request.RemoteAddr supplies) is allowed to set X-Forwarded-Proto
// and have us honor it for canonical-cookie selection.
//
// The allowlist is configured via TRUSTED_PROXY_CIDRS — a comma-separated
// list of CIDR ranges. An empty (or unset) list means "no proxy is trusted"
// — the safe default for local dev where we rely on r.TLS only.
//
// Deployment requirement: the configured ingress proxy MUST strip
// client-supplied X-Forwarded-Proto headers before forwarding. Allowlist
// alone is not sufficient — without stripping, a malicious request that
// reaches the trusted proxy can still inject scheme metadata.
func IsTrustedProxy(remoteAddr string) bool {
	cidrs := loadTrustedCIDRs()
	if len(cidrs) == 0 {
		return false
	}
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

var (
	cidrCacheValue []*net.IPNet
	cidrCacheRaw   string
	cidrCacheMu    sync.RWMutex
)

// loadTrustedCIDRs parses TRUSTED_PROXY_CIDRS, re-parsing only when the
// env var actually changes (so tests can flip it via t.Setenv without
// restart). Bad CIDR entries are silently dropped — they're operator-
// supplied and a typo should fail closed rather than panic the process.
func loadTrustedCIDRs() []*net.IPNet {
	raw := os.Getenv("TRUSTED_PROXY_CIDRS")

	cidrCacheMu.RLock()
	if raw == cidrCacheRaw && cidrCacheValue != nil {
		v := cidrCacheValue
		cidrCacheMu.RUnlock()
		return v
	}
	cidrCacheMu.RUnlock()

	parsed := parseTrustedCIDRs(raw)

	cidrCacheMu.Lock()
	cidrCacheRaw = raw
	cidrCacheValue = parsed
	cidrCacheMu.Unlock()
	return parsed
}

func parseTrustedCIDRs(raw string) []*net.IPNet {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]*net.IPNet, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		_, n, err := net.ParseCIDR(p)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

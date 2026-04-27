package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTrustedProxy_EmptyAllowlist_RejectsAll(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	assert.False(t, IsTrustedProxy("127.0.0.1:1234"))
	assert.False(t, IsTrustedProxy("10.0.0.1:80"))
	assert.False(t, IsTrustedProxy("8.8.8.8:443"))
}

func TestIsTrustedProxy_LocalhostInRange(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.0/8")
	assert.True(t, IsTrustedProxy("127.0.0.1:1234"))
	assert.True(t, IsTrustedProxy("127.5.5.5:80"))
	assert.False(t, IsTrustedProxy("128.0.0.1:80"))
}

func TestIsTrustedProxy_MultipleCIDRs(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.0.0/16")
	assert.True(t, IsTrustedProxy("10.5.5.5:0"))
	assert.True(t, IsTrustedProxy("192.168.1.1:443"))
	assert.False(t, IsTrustedProxy("172.16.0.1:80"))
	assert.False(t, IsTrustedProxy("8.8.8.8:80"))
}

func TestIsTrustedProxy_IPv6(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "::1/128, fd00::/8")
	assert.True(t, IsTrustedProxy("[::1]:1234"))
	assert.True(t, IsTrustedProxy("[fd12::1]:80"))
	assert.False(t, IsTrustedProxy("[2001:db8::1]:80"))
}

func TestIsTrustedProxy_BadCIDR_SilentlyDropped(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr, 10.0.0.0/8, also-bad")
	// The valid entry still works; bad entries are dropped.
	assert.True(t, IsTrustedProxy("10.1.2.3:0"))
}

func TestIsTrustedProxy_BadRemoteAddr(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	assert.False(t, IsTrustedProxy("garbage"))
	assert.False(t, IsTrustedProxy(""))
}

func TestIsTrustedProxy_HostOnlyAddress(t *testing.T) {
	// http.Request.RemoteAddr usually has "host:port" but not always — be
	// permissive about no-port hostnames too.
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	assert.True(t, IsTrustedProxy("10.5.5.5"))
	assert.False(t, IsTrustedProxy("8.8.8.8"))
}

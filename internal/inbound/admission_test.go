package inbound

import (
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"pgregory.net/rapid"
)

// stringAddr is a net.Addr with a caller-controlled string representation.
type stringAddr string

func (a stringAddr) Network() string { return "tcp" }
func (a stringAddr) String() string  { return string(a) }

// TestInboundSourcePrefix verifies source normalization across address forms.
func TestInboundSourcePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		addr net.Addr
		want netip.Prefix
	}{
		{
			name: "ipv4",
			addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.99"), Port: 1},
			want: netip.MustParsePrefix("192.0.2.0/24"),
		},
		{
			name: "ipv4 mapped",
			addr: stringAddr("[::ffff:192.0.2.99]:8333"),
			want: netip.MustParsePrefix("192.0.2.0/24"),
		},
		{
			name: "ipv6",
			addr: &net.TCPAddr{
				IP:   net.ParseIP("2001:db8:1:2:3:4:5:6"),
				Port: 8333,
			},
			want: netip.MustParsePrefix("2001:db8:1:2::/64"),
		},
		{
			name: "ipv6 zone",
			addr: stringAddr("[fe80::1234%en0]:8333"),
			want: netip.MustParsePrefix("fe80::/64"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := inboundSourcePrefix(test.addr)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

// TestInboundSourcePrefixIgnoresPort verifies that ports cannot create new
// source-accounting entries.
func TestInboundSourcePrefixIgnoresPort(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		portA := rapid.IntRange(0, 65535).Draw(t, "port_a")
		portB := rapid.IntRange(0, 65535).Draw(t, "port_b")
		ip := net.IPv4(
			rapid.Byte().Draw(t, "a"),
			rapid.Byte().Draw(t, "b"),
			rapid.Byte().Draw(t, "c"),
			rapid.Byte().Draw(t, "d"),
		)

		prefixA, err := inboundSourcePrefix(&net.TCPAddr{
			IP: ip, Port: portA,
		})
		require.NoError(t, err)
		prefixB, err := inboundSourcePrefix(&net.TCPAddr{
			IP: ip, Port: portB,
		})
		require.NoError(t, err)
		require.Equal(t, prefixA, prefixB)
	})
}

// TestIsLoopback verifies loopback detection across supported address forms.
func TestIsLoopback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		addr net.Addr
		want bool
	}{
		{
			name: "ipv4 loopback",
			addr: &net.TCPAddr{
				IP: net.ParseIP("127.0.0.2"), Port: 8333,
			},
			want: true,
		},
		{
			name: "ipv6 loopback",
			addr: stringAddr("[::1]:8333"),
			want: true,
		},
		{
			name: "public",
			addr: stringAddr("192.0.2.1:8333"),
		},
		{
			name: "malformed",
			addr: stringAddr("attacker-controlled"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, IsLoopback(test.addr))
		})
	}
}

// TestInboundSourceAdmission verifies independent per-prefix limits, exact
// release semantics, and map cleanup.
func TestInboundSourceAdmission(t *testing.T) {
	t.Parallel()

	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 2,
		v2Rate:              rate.Inf,
		v2SourceRate:        rate.Inf,
		v2SourceCacheSize:   16,
		v2Concurrency:       1,
	})

	sourceA1 := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1}
	sourceA2 := &net.TCPAddr{IP: net.ParseIP("192.0.2.200"), Port: 2}
	sourceB := &net.TCPAddr{IP: net.ParseIP("192.0.3.1"), Port: 3}

	releaseA1, err := admission.AcquireSource(sourceA1, false)
	require.NoError(t, err)
	releaseA2, err := admission.AcquireSource(sourceA2, false)
	require.NoError(t, err)

	_, err = admission.AcquireSource(sourceA1, false)
	require.ErrorIs(t, err, errInboundSourceLimit)

	releaseB, err := admission.AcquireSource(sourceB, false)
	require.NoError(t, err, "an independent prefix should remain admissible")

	releaseA1()
	releaseA1()
	replacement, err := admission.AcquireSource(sourceA1, false)
	require.NoError(t, err, "double release must only free one slot")

	releaseA2()
	replacement()
	releaseB()

	admission.mu.Lock()
	defer admission.mu.Unlock()
	require.Empty(t, admission.pendingBySource)
}

// TestInboundSourceAdmissionMalformedAddress verifies malformed addresses fail
// closed without allocating map entries.
func TestInboundSourceAdmissionMalformedAddress(t *testing.T) {
	t.Parallel()

	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 2,
		v2Rate:              rate.Inf,
		v2SourceRate:        rate.Inf,
		v2SourceCacheSize:   16,
		v2Concurrency:       1,
	})

	_, err := admission.AcquireSource(
		stringAddr("attacker-controlled"), false,
	)
	require.Error(t, err)
	require.Empty(t, admission.pendingBySource)
}

// TestBypassSourceLimits verifies trusted sources bypass only per-source
// accounting while the global v2 rate and concurrency budgets remain active.
func TestBypassSourceLimits(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	trusted := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 8333}
	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 1,
		v2Rate:              2,
		v2Burst:             2,
		v2SourceRate:        1,
		v2SourceBurst:       1,
		v2SourceCacheSize:   16,
		v2Concurrency:       2,
		now:                 func() time.Time { return now },
	})

	releaseSource1, err := admission.AcquireSource(trusted, true)
	require.NoError(t, err)
	releaseSource2, err := admission.AcquireSource(trusted, true)
	require.NoError(t, err)
	releaseSource1()
	releaseSource1()
	releaseSource2()
	require.Empty(t, admission.pendingBySource)

	releaseV2First, err := admission.BindV2(trusted, true).Acquire()
	require.NoError(t, err)
	releaseV2Second, err := admission.BindV2(trusted, true).Acquire()
	require.NoError(t, err)

	_, err = admission.BindV2(trusted, true).Acquire()
	require.ErrorIs(t, err, errV2HandshakeRateLimit)

	now = now.Add(500 * time.Millisecond)
	_, err = admission.BindV2(trusted, true).Acquire()
	require.ErrorIs(t, err, errV2HandshakeConcurrency)

	releaseV2First()
	releaseV2Second()
}

// TestV2HandshakeAdmission verifies deterministic token refill and concurrent
// crypto-slot admission.
func TestV2HandshakeAdmission(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 8333}
	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 1,
		v2Rate:              2,
		v2Burst:             2,
		v2SourceRate:        rate.Inf,
		v2SourceCacheSize:   16,
		v2Concurrency:       2,
		now:                 func() time.Time { return now },
	})

	release1, err := admission.BindV2(remote, false).Acquire()
	require.NoError(t, err)
	release2, err := admission.BindV2(remote, false).Acquire()
	require.NoError(t, err)

	_, err = admission.BindV2(remote, false).Acquire()
	require.ErrorIs(t, err, errV2HandshakeRateLimit)

	release1()
	release2()
	now = now.Add(500 * time.Millisecond)

	release3, err := admission.BindV2(remote, false).Acquire()
	require.NoError(t, err, "one token should refill after half a second")
	release3()

	_, err = admission.BindV2(remote, false).Acquire()
	require.ErrorIs(t, err, errV2HandshakeRateLimit)
}

// TestV2HandshakeConcurrency verifies that crypto slots are released exactly
// once and do not wait when all slots are occupied.
func TestV2HandshakeConcurrency(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	sourceA := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1}
	sourceB := &net.TCPAddr{IP: net.ParseIP("192.0.3.1"), Port: 2}
	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 1,
		v2Rate:              1,
		v2Burst:             2,
		v2SourceRate:        1,
		v2SourceBurst:       1,
		v2SourceCacheSize:   16,
		v2Concurrency:       1,
		now:                 func() time.Time { return now },
	})

	release, err := admission.BindV2(sourceA, false).Acquire()
	require.NoError(t, err)
	_, err = admission.BindV2(sourceB, false).Acquire()
	require.True(t, errors.Is(err, errV2HandshakeConcurrency))

	release()
	release()
	replacement, err := admission.BindV2(sourceB, false).Acquire()
	require.NoError(t, err)
	replacement()
}

// TestV2HandshakeGlobalRateRollback verifies a global rejection does not
// consume the rejected source's independent token.
func TestV2HandshakeGlobalRateRollback(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 1,
		v2Rate:              1,
		v2Burst:             1,
		v2SourceRate:        0.01,
		v2SourceBurst:       1,
		v2SourceCacheSize:   16,
		v2Concurrency:       1,
		now:                 func() time.Time { return now },
	})

	sourceA := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1}
	sourceB := &net.TCPAddr{IP: net.ParseIP("192.0.3.1"), Port: 2}

	release, err := admission.BindV2(sourceA, false).Acquire()
	require.NoError(t, err)
	release()

	_, err = admission.BindV2(sourceB, false).Acquire()
	require.ErrorIs(t, err, errV2HandshakeRateLimit)

	now = now.Add(time.Second)
	release, err = admission.BindV2(sourceB, false).Acquire()
	require.NoError(t, err,
		"a global rejection must return the source reservation")
	release()
}

// TestV2HandshakeSourceRate verifies one source prefix cannot consume the
// global responder budget or prevent another source from being admitted.
func TestV2HandshakeSourceRate(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	admission := newAdmission(admissionConfig{
		maxPendingPerSource: 1,
		v2Rate:              rate.Inf,
		v2SourceRate:        1,
		v2SourceBurst:       2,
		v2SourceCacheSize:   16,
		v2Concurrency:       1,
		now:                 func() time.Time { return now },
	})

	sourceA := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1}
	sourceASamePrefix := &net.TCPAddr{
		IP: net.ParseIP("192.0.2.200"), Port: 2,
	}
	sourceB := &net.TCPAddr{IP: net.ParseIP("192.0.3.1"), Port: 3}

	for _, source := range []net.Addr{sourceA, sourceASamePrefix} {
		release, err := admission.BindV2(source, false).Acquire()
		require.NoError(t, err)
		release()
	}

	_, err := admission.BindV2(sourceA, false).Acquire()
	require.ErrorIs(t, err, errV2HandshakeSourceRateLimit)

	release, err := admission.BindV2(sourceB, false).Acquire()
	require.NoError(t, err,
		"an independent source prefix should retain its own budget")
	release()
}

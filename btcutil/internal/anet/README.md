# anet (vendored)

This package is a vendored copy of an Android networking shim. It works around
[golang/go#40569](https://github.com/golang/go/issues/40569), where
`net.Interfaces()` and `net.InterfaceAddrs()` fail with
`route ip+net: netlinkrib: permission denied` on Android 11 and later due to the
tightened restrictions on `NETLINK` sockets (no `bind`, no `RTM_GETLINK`).

`btcutil` only relies on `anet.InterfaceAddrs()`, which is wired in from
`net_android.go` under the `android` build tag.

## Provenance

The source originates from `github.com/wlynxg/anet` (BSD 3-Clause, see
`LICENSE`). It was vendored here, rather than imported as a module dependency,
so that `btcd` does not depend on a fork hosted under a personal account.

Unlike upstream, this copy does **not** use the `//go:linkname` mechanism to
reach into the standard library's `net` package; it carries its own
`ipv6ZoneCache`. As a result, no `-ldflags "-checklinkname=0"` linker flag is
required to build it.

When [golang/go#40569](https://github.com/golang/go/issues/40569) is resolved
upstream, this package and the `net_android.go` shim can be removed.

integration
===========

[![Build Status](https://github.com/btcsuite/btcd/workflows/Build%20and%20Test/badge.svg)](https://github.com/btcsuite/btcd/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

This contains integration tests which make use of the
[rpctest](https://github.com/btcsuite/btcd/tree/master/integration/rpctest)
package to programmatically drive nodes via RPC.

## Browser WebTransport test

The WebTransport test builds btcd's peer package as Go/WASM, runs it in a
stock Chrome or Chromium browser, and connects it across origins to a native
btcd simnet node whose WebTransport listener allows every HTTP(S) browser
origin:

```bash
go test -v -tags=rpctest ./integration \
  -run '^TestWebTransportBrowserWASMPeer$' -count=1
```

Set `BTCD_CHROME_BIN` if Chrome is not on a standard path.  The test skips when
no browser is available.  It does not bypass browser certificate validation;
the test server uses a short-lived P-256 certificate pinned by its SHA-256
hash.

## License

This code is licensed under the [copyfree](http://copyfree.org) ISC License.

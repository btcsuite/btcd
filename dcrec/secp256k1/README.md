secp256k1
=====

[![Build Status](http://img.shields.io/travis/decred/dcrd.svg)](https://travis-ci.org/decred/dcrd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/decred/dcrd/dcrec/secp256k1)

Package dcrec implements elliptic curve cryptography needed for working with
Decred (secp256k1 only for now). It is designed so that it may be used with the
standard crypto/ecdsa packages provided with go.  A comprehensive suite of test
is provided to ensure proper functionality.  Package dcrec was originally based
on work from ThePiachu which is licensed under the same terms as Go, but it has
signficantly diverged since then.  The decred developers original is licensed
under the liberal ISC license.

Although this package was primarily written for dcrd, it has intentionally been
designed so it can be used as a standalone package for any projects needing to
use secp256k1 elliptic curve cryptography.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/dcrec
```

## Examples

* [Sign Message](http://godoc.org/github.com/decred/dcrd/dcrec#example-package--SignMessage)  
  Demonstrates signing a message with a secp256k1 private key that is first
  parsed form raw bytes and serializing the generated signature.

* [Verify Signature](http://godoc.org/github.com/decred/dcrd/dcrec#example-package--VerifySignature)  
  Demonstrates verifying a secp256k1 signature against a public key that is
  first parsed from raw bytes.  The signature is also parsed from raw bytes.

* [Encryption](http://godoc.org/github.com/decred/dcrd/dcrec#example-package--EncryptMessage)  
  Demonstrates encrypting a message for a public key that is first parsed from
  raw bytes, then decrypting it using the corresponding private key.

* [Decryption](http://godoc.org/github.com/decred/dcrdy/dcrec#example-package--DecryptMessage)  
  Demonstrates decrypting a message using a private key that is first parsed
  from raw bytes.

## License

Package dcrec is licensed under the [copyfree](http://copyfree.org) ISC License
except for dcrec.go and dcrec_test.go which is under the same license as Go.


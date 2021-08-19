btcec
=====

[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

btcec implements elliptic curve cryptography needed for working with
Bitcoin (secp256k1 only for now). It is designed so that it may be used with the
standard crypto/ecdsa packages provided with go.  A comprehensive suite of test
is provided to ensure proper functionality.  Package btcec was originally based
on work from ThePiachu which is licensed under the same terms as Go, but it has
signficantly diverged since then.
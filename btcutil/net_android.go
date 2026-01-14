// Copyright (c) 2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build android
// +build android

package btcutil

import (
	"net"

	"github.com/kcalvinalvin/anet"
)

// InterfaceAddrs returns a list of the system's network interface addresses.
// We specifically wrap anet.InterfaceAddrs as the net pakcage has been broken
// since android 11.
//
// The relevant github issue link is:
// https://github.com/golang/go/issues/40569.
// When the issue is later resolved, we should remove it.
func InterfaceAddrs() ([]net.Addr, error) {
	return anet.InterfaceAddrs()
}

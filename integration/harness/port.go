// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package harness

// NetPortManager is used by the test setup to manage issuing
// a new network port number
type NetPortManager interface {
	// ObtainPort provides a new network port number upon request.
	ObtainPort() int
}

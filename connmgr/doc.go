// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package connmgr implements a generic Bitcoin network connection manager.

Connection Manager Overview

Connection Manager handles all the general connection concerns such as
maintaining a fixed number of outbound connections, sourcing peers from seeds
and the address manager, banning misbehaving connections, limiting max
connections based on configuration, routing connections via tor etc.

*/
package connmgr

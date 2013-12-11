btcws
=====

[![Build Status](https://travis-ci.org/conformal/btcws.png?branch=master)]
(https://travis-ci.org/conformal/btcws)

Package btcws implements extensions to the standard bitcoind JSON-RPC
API for the btcd suite of programs (btcd, btcwallet, and btcgui).
Importing this package registers all implemented custom requests with
btcjson (using btcjson.RegisterCustomCmd).

## Sample Use
```Go
// Client Side
import (
	"code.google.com/p/go.net/websocket"
	"github.com/conformal/btcws"
)

// Create rescan command.
id := 0
addrs := map[string]struct{}{
	"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH": struct{},
}
cmd, err := btcws.NewRescanCmd(id, 270000, addrs)

// Set up a handler for a reply with id 0.
AddReplyHandler(id, func(reply map[string]interface{}) {
	// Deal with reply.
})

// JSON marshal and send rescan request to websocket connection.
websocket.JSON.Send(btcdWSConn, cmd)


// Server Side
import (
	"code.google.com/p/go.net/websocket"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcws"
)

// Get marshaled request.
var b []byte
err := websocket.Message.Receive(clientWSConn, &b)

// Parse marshaled command.
cmd, err := btcjson.ParseMarshaledCmd(b)

// If this is a rescan command, handle and reply.
rcmd, ok := cmd.(*btcws.RescanCmd)
if ok {
	// Do stuff
	var reply []byte
	err := websocket.Message.Send(clientWSConn, reply)
}

```

## Installation

```bash
$ go get github.com/conformal/btcws
```

## GPG Verification Key

All official release tags are signed by Conformal so users can ensure the code
has not been tampered with and is coming from Conformal.  To verify the
signature perform the following:

- Download the public key from the Conformal website at
  https://opensource.conformal.com/GIT-GPG-KEY-conformal.txt

- Import the public key into your GPG keyring:
  ```bash
  gpg --import GIT-GPG-KEY-conformal.txt
  ```

- Verify the release tag with the following command where `TAG_NAME` is a
  placeholder for the specific tag:
  ```bash
  git tag -v TAG_NAME
  ```

## License

Package btcws is licensed under the liberal ISC License.

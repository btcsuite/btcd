// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"bytes"
	_ "crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

// MarshallAndSend takes the reply structure, marshalls it to json, and
// sends it back to the io.Writer (most likely an http.ResponseWriter).
// returning a log message and an error if there is one.
func MarshallAndSend(rawReply Reply, w io.Writer) (string, error) {
	finalReply, err := json.Marshal(rawReply)
	if err != nil {
		msg := fmt.Sprintf("[RPCS] Error Marshalling reply: %v", err)
		return msg, err
	}
	fmt.Fprintf(w, "%s\n", finalReply)
	msg := fmt.Sprintf("[RPCS] reply: %v", rawReply)
	return msg, nil
}

// jsonRpcSend connects to the daemon with the specified username, password,
// and ip/port and then send the supplied message.  This uses net/http rather
// than net/rpc/jsonrpc since that one doesn't support http connections and is
// therefore useless.
func jsonRpcSend(user string, password string, server string, message []byte,
	https bool, certificates []byte, skipverify bool) (*http.Response, error) {
	client := &http.Client{}
	protocol := "http"
	if https {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(certificates)

		config := &tls.Config{
			InsecureSkipVerify: skipverify,
			RootCAs:            pool,
		}
		transport := &http.Transport{TLSClientConfig: config}
		client.Transport = transport
		protocol = "https"
	}
	credentials := user + ":" + password
	resp, err := client.Post(protocol+"://"+credentials+"@"+server,
		"application/json", bytes.NewReader(message))
	if err != nil {
		// We do not want to log the username/password in the errors.
		replaceStr := "<username>:<password>"
		str := strings.Replace(err.Error(), credentials, replaceStr, -1)
		err = fmt.Errorf("%v", str)
	}
	return resp, err
}

// GetRaw should be called after JsonRpcSend.  It reads and returns
// the reply (which you can then call ReadResultCmd() on) and closes the
// connection.
func GetRaw(resp io.ReadCloser) ([]byte, error) {
	body, err := ioutil.ReadAll(resp)
	resp.Close()
	if err != nil {
		err = fmt.Errorf("error reading json reply: %v", err)
		return body, err
	}
	return body, nil
}

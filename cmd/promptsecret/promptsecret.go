// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

func zero(b []byte) {
	for i := 0; i < len(b); i++ {
		b[i] = 0x00
	}
}

func main() {
	fmt.Fprint(os.Stderr, "Secret: ")

	secret, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprint(os.Stderr, "\n")
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read secret: %v\n", err)
		os.Exit(1)
	}

	_, err = os.Stdout.Write(secret)
	zero(secret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
		os.Exit(1)
	}
}

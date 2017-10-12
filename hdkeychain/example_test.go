// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package hdkeychain_test

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/hdkeychain"
)

// This example demonstrates how to generate a cryptographically random seed
// then use it to create a new master node (extended key).
func Example_newMaster() {
	// Generate a random seed at the recommended length.
	seed, err := hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Generate a new master node using the seed.
	key, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Show that the generated master node extended key is private.
	fmt.Println("Private Extended Key?:", key.IsPrivate())

	// Output:
	// Private Extended Key?: true
}

// This example demonstrates the default hierarchical deterministic wallet
// layout as described in BIP0032.
func Example_defaultWalletLayout() {
	// The default wallet layout described in BIP0032 is:
	//
	// Each account is composed of two keypair chains: an internal and an
	// external one. The external keychain is used to generate new public
	// addresses, while the internal keychain is used for all other
	// operations (change addresses, generation addresses, ..., anything
	// that doesn't need to be communicated).
	//
	//   * m/iH/0/k
	//     corresponds to the k'th keypair of the external chain of account
	//     number i of the HDW derived from master m.
	//   * m/iH/1/k
	//     corresponds to the k'th keypair of the internal chain of account
	//     number i of the HDW derived from master m.

	// Ordinarily this would either be read from some encrypted source
	// and be decrypted or generated as the NewMaster example shows, but
	// for the purposes of this example, the private exteded key for the
	// master node is being hard coded here.
	master := "dprv3hCznBesA6jBushjx7y9NrfheE4ZshnaKYtsoLXefmLPzrXgEiXkd" +
		"RMD6UngnmBYZzgNhdEd4K3PidxcaCiR6HC9hmpj8FcrP4Cv7zBwELA"

	// Start by getting an extended key instance for the master node.
	// This gives the path:
	//   m
	masterKey, err := hdkeychain.NewKeyFromString(master)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the extended key for account 0.  This gives the path:
	//   m/0H
	acct0, err := masterKey.Child(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the extended key for the account 0 external chain.  This
	// gives the path:
	//   m/0H/0
	acct0Ext, err := acct0.Child(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the extended key for the account 0 internal chain.  This gives
	// the path:
	//   m/0H/1
	acct0Int, err := acct0.Child(1)
	if err != nil {
		fmt.Println(err)
		return
	}

	// At this point, acct0Ext and acct0Int are ready to derive the keys for
	// the external and internal wallet chains.

	// Derive the 10th extended key for the account 0 external chain.  This
	// gives the path:
	//   m/0H/0/10
	acct0Ext10, err := acct0Ext.Child(10)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the 1st extended key for the account 0 internal chain.  This
	// gives the path:
	//   m/0H/1/0
	acct0Int0, err := acct0Int.Child(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Get and show the address associated with the extended keys for the
	// main decred network.
	acct0ExtAddr, err := acct0Ext10.Address(&chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return
	}
	acct0IntAddr, err := acct0Int0.Address(&chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Account 0 External Address 10:", acct0ExtAddr)
	fmt.Println("Account 0 Internal Address 0:", acct0IntAddr)

	// Output:
	// Account 0 External Address 10: DshMmJ3bfvMDdk1mkXRD3x5xDuPwSxoYGfi
	// Account 0 Internal Address 0: DsoTyktAyEDkYpgKSex6zx5rrkFDi2gAsHr
}

// This example demonstrates the audits use case in BIP0032.
func Example_audits() {
	// The audits use case described in BIP0032 is:
	//
	// In case an auditor needs full access to the list of incoming and
	// outgoing payments, one can share all account public extended keys.
	// This will allow the auditor to see all transactions from and to the
	// wallet, in all accounts, but not a single secret key.
	//
	//   * N(m/*)
	//   corresponds to the neutered master extended key (also called
	//   the master public extended key)

	// Ordinarily this would either be read from some encrypted source
	// and be decrypted or generated as the NewMaster example shows, but
	// for the purposes of this example, the private exteded key for the
	// master node is being hard coded here.
	master := "dprv3hCznBesA6jBushjx7y9NrfheE4ZshnaKYtsoLXefmLPzrXgEiXkd" +
		"RMD6UngnmBYZzgNhdEd4K3PidxcaCiR6HC9hmpj8FcrP4Cv7zBwELA"

	// Start by getting an extended key instance for the master node.
	// This gives the path:
	//   m
	masterKey, err := hdkeychain.NewKeyFromString(master)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Neuter the master key to generate a master public extended key.  This
	// gives the path:
	//   N(m/*)
	masterPubKey, err := masterKey.Neuter()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Share the master public extended key with the auditor.
	mpks, err := masterPubKey.String()
	if err != nil {
		panic("unexpected error creating string of extended public key")
	}
	fmt.Println("Audit key N(m/*):", mpks)

	// Output:
	// Audit key N(m/*): dpubZ9169KDAEUnypHbWCe2Vu5TxGEcqJeNeX6XCYFU1fqw2iQZK7fsMhzsEFArbLmyUdprUw9aXHneUNd92bjc31TqC6sUduMY6PK2z4JXDS8j
}

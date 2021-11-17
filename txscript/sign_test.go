// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

type addressToKey struct {
	key        *btcec.PrivateKey
	compressed bool
}

func mkGetKey(keys map[string]addressToKey) KeyDB {
	if keys == nil {
		return KeyClosure(func(addr btcutil.Address) (*btcec.PrivateKey,
			bool, error) {
			return nil, false, errors.New("nope")
		})
	}
	return KeyClosure(func(addr btcutil.Address) (*btcec.PrivateKey,
		bool, error) {
		a2k, ok := keys[addr.EncodeAddress()]
		if !ok {
			return nil, false, errors.New("nope")
		}
		return a2k.key, a2k.compressed, nil
	})
}

func mkGetScript(scripts map[string][]byte) ScriptDB {
	if scripts == nil {
		return ScriptClosure(func(addr btcutil.Address) ([]byte, error) {
			return nil, errors.New("nope")
		})
	}
	return ScriptClosure(func(addr btcutil.Address) ([]byte, error) {
		script, ok := scripts[addr.EncodeAddress()]
		if !ok {
			return nil, errors.New("nope")
		}
		return script, nil
	})
}

func checkScripts(msg string, tx *wire.MsgTx, idx int, inputAmt int64, sigScript, pkScript []byte) error {
	tx.TxIn[idx].SignatureScript = sigScript
	vm, err := NewEngine(pkScript, tx, idx,
		ScriptBip16|ScriptVerifyDERSignatures, nil, nil, inputAmt)
	if err != nil {
		return fmt.Errorf("failed to make script engine for %s: %v",
			msg, err)
	}

	err = vm.Execute()
	if err != nil {
		return fmt.Errorf("invalid script signature for %s: %v", msg,
			err)
	}

	return nil
}

func signAndCheck(msg string, tx *wire.MsgTx, idx int, inputAmt int64, pkScript []byte,
	hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte) error {

	sigScript, err := SignTxOutput(&chaincfg.TestNet3Params, tx, idx,
		pkScript, hashType, kdb, sdb, nil)
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}

	err = checkScripts(msg, tx, idx, inputAmt, sigScript, pkScript)
	if err != nil {
		return fmt.Errorf("failed to verify output %s: %v", msg, err)
	}

	sigHashes := NewTxSigHashes(tx)
	sigScript, _, err = SignTxWitness(&chaincfg.TestNet3Params, tx, sigHashes, idx,
		pkScript, inputAmt, hashType, kdb, sdb, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to sign output with SignTxWitness %s: %v", msg, err)
	}

	err = checkScriptsAndWitness(msg, tx, sigHashes, idx, inputAmt, sigScript, pkScript, nil)
	if err != nil {
		return fmt.Errorf("failed to verify output %s: %v", msg, err)
	}
	return nil
}

func checkScriptsAndWitness(msg string, tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	inputAmt int64, sigScript, pkScript []byte, witness wire.TxWitness) error {
	tx.TxIn[idx].SignatureScript = sigScript
	tx.TxIn[idx].Witness = witness
	vm, err := NewEngine(pkScript, tx, idx,
		ScriptBip16|ScriptVerifyWitness|ScriptVerifyDERSignatures, nil, sigHashes, inputAmt)
	if err != nil {
		return fmt.Errorf("failed to make script engine for %s: %v",
			msg, err)
	}

	err = vm.Execute()
	if err != nil {
		return fmt.Errorf("invalid script signature for %s: %v", msg,
			err)
	}

	return nil
}

func signAndCheckWitness(msg string, tx *wire.MsgTx, sigHashes *TxSigHashes, idx int,
	inputAmt int64, pkScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte) error {

	sigScript, wit, err := SignTxWitness(&chaincfg.TestNet3Params, tx, sigHashes, idx,
		pkScript, inputAmt, hashType, kdb, sdb, previousScript, nil)
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}

	return checkScriptsAndWitness(msg, tx, sigHashes, idx, inputAmt, sigScript, pkScript, wit)
}

func TestSignTxOutput(t *testing.T) {
	t.Parallel()

	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []SigHashType{
		SigHashOld, // no longer used but should act like all
		SigHashAll,
		SigHashNone,
		SigHashSingle,
		SigHashAll | SigHashAnyOneCanPay,
		SigHashNone | SigHashAnyOneCanPay,
		SigHashSingle | SigHashAnyOneCanPay,
	}
	inputAmounts := []int64{5, 10, 15}
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0,
				},
				Sequence: 4294967295,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 1,
				},
				Sequence: 4294967295,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 2,
				},
				Sequence: 4294967295,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
			{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	// Pay to Pubkey Hash (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i], pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (uncompressed) (merging with correct)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i], sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i], sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, pkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// As before, but with p2sh now.
	// Pay to Pubkey Hash (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
				break
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (uncompressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)
			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
				break
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to Pubkey Hash (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(
				scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed) with duplicate merge
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeUncompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, false},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressPubKey(pk,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// by the above loop, this should be valid, now sign
			// again and merge.
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s a "+
					"second time: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("twice signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Basic Multisig
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			pkScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			if err := signAndCheck(msg, tx, i, inputAmounts[i],
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Two part multisig, sign with one key then the other.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			pkScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, inputAmounts[i], sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, inputAmounts[i], sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Two part multisig, sign with one key then both, check key dedup
	// correctly.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			pkScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := btcutil.NewAddressScriptHash(
				pkScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			scriptPkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, err := SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, inputAmounts[i], sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = SignTxOutput(&chaincfg.TestNet3Params,
				tx, i, scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): pkScript,
				}), sigScript)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			// Now we should pass.
			err = checkScripts(msg, tx, i, inputAmounts[i],
				sigScript, scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// test segwit scripts are rejected for not providing sigHashes
	key, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		t.Fatalf("failed to make privKey: %v", err)
	}

	pk1 := (*btcec.PublicKey)(&key.PublicKey).
		SerializeCompressed()

	p2wpkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(
		btcutil.Hash160(pk1), &chaincfg.TestNet3Params)
	if err != nil {
		t.Fatalf("failed to make p2wpkh address %v", err)
	}

	p2wpkhScript, err := PayToAddrScript(p2wpkhAddr)
	if err != nil {
		t.Fatalf("failed to make p2wpkh script %v", err)
	}

	_, err = SignTxOutput(&chaincfg.TestNet3Params,
		tx, 0, p2wpkhScript, SigHashAll,
		mkGetKey(map[string]addressToKey{
			p2wpkhAddr.EncodeAddress(): {key, true},
		}), mkGetScript(nil), nil)
	if err == nil {
		t.Fatal("Error expected")
	}
	if err.Error() != "cannot sign segwit outputs without TxSigHashes" {
		t.Fatalf("Unexpected error %v", err)
	}
}

func TestSignTxOutputWitness(t *testing.T) {
	t.Parallel()

	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []SigHashType{
		//SigHashOld, // no longer used but should act like all
		SigHashAll,
		SigHashNone,
		SigHashSingle,
		SigHashAll | SigHashAnyOneCanPay,
		SigHashNone | SigHashAnyOneCanPay,
		SigHashSingle | SigHashAnyOneCanPay,
	}
	inputAmounts := []int64{5, 10, 15}
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0,
				},
				Sequence: 4294967295,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 1,
				},
				Sequence: 4294967295,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 2,
				},
				Sequence: 4294967295,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
			{
				Value: 3,
			},
		},
		LockTime: 0,
	}

	sigHashes := NewTxSigHashes(tx)

	// Pay to witness pubkey hash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			address, err := btcutil.NewAddressWitnessPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
				break
			}

			// call functions directly here as we're going to
			// verify WitnessSignature's witness later
			sigScript, wit, err := SignTxWitness(&chaincfg.TestNet3Params, tx, sigHashes, i,
				pkScript, inputAmounts[i], hashType, mkGetKey(map[string]addressToKey{
					address.EncodeAddress(): {key, true},
				}), mkGetScript(nil), nil, nil)
			if err != nil {
				t.Errorf("failed to make p2wpkh signature "+
					"for %s: %v", msg, err)
				break
			}

			err = checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i], sigScript, pkScript, wit)
			if err != nil {
				t.Errorf("failed to verify p2wpkh signature "+
					"for %s: %v", msg, err)
				break
			}

			witness, err := WitnessSignature(tx, sigHashes, i, inputAmounts[i], pkScript, hashType, key, true)
			if err != nil {
				t.Errorf("WitnessSignature failed on valid inputs "+
					"for %s: %v", msg, err)
				break
			}
			if len(wit) != len(witness) {
				t.Errorf("WitnessSignature produced invalid witness "+
					"for %s: %v", msg, err)
				break
			}
			for i := 0; i < len(wit); i++ {
				if !bytes.Equal(wit[i], witness[i]) {
					t.Fatalf("WitnessSignature elements dont match %s %s "+
						"for %s: %v", hex.EncodeToString(wit[i]), hex.EncodeToString(witness[i]),
						msg, err)
				}
			}
		}
	}

	// P2SH Pay to witness pubkey hash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			wprog, err := btcutil.NewAddressWitnessPubKeyHash(
				btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}
			redeemScript, err := PayToAddrScript(wprog)
			if err != nil {
				t.Errorf("failed to make redeemScript for %s: %v",
					msg, err)
				break
			}
			address, err := btcutil.NewAddressScriptHashFromHash(btcutil.Hash160(redeemScript), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					wprog.EncodeAddress(): {key, true},
				}),
				mkGetScript(map[string][]byte{
					address.EncodeAddress(): redeemScript,
				}),
				nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// P2WSH Pay to public key (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			pubKeyAddr, err := btcutil.NewAddressPubKey(pk, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}
			witnessScript, err := PayToAddrScript(pubKeyAddr)
			if err != nil {
				t.Errorf("failed to make redeemScript for %s: %v",
					msg, err)
				break
			}
			wsHash := sha256.Sum256(witnessScript)
			address, err := btcutil.NewAddressWitnessScriptHash(wsHash[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					pubKeyAddr.EncodeAddress(): {key, true},
				}),
				mkGetScript(map[string][]byte{
					address.EncodeAddress(): witnessScript,
				}),
				nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// P2WSH Pay to pubkeyhash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			pubKeyAddr, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}
			witnessScript, err := PayToAddrScript(pubKeyAddr)
			if err != nil {
				t.Errorf("failed to make redeemScript for %s: %v",
					msg, err)
				break
			}
			wsHash := sha256.Sum256(witnessScript)
			address, err := btcutil.NewAddressWitnessScriptHash(wsHash[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(address)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					pubKeyAddr.EncodeAddress(): {key, true},
				}),
				mkGetScript(map[string][]byte{
					address.EncodeAddress(): witnessScript,
				}),
				nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// P2SH P2WSH Pay to pubkey (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			pubKeyAddr, err := btcutil.NewAddressPubKey(pk, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}
			witnessScript, err := PayToAddrScript(pubKeyAddr)
			if err != nil {
				t.Errorf("failed to make redeemScript for %s: %v",
					msg, err)
				break
			}
			wsHash := sha256.Sum256(witnessScript)
			wpProg, err := btcutil.NewAddressWitnessScriptHash(wsHash[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			redeemScript, err := PayToAddrScript(wpProg)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}
			p2shAddr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			pkScript, err := PayToAddrScript(p2shAddr)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					pubKeyAddr.EncodeAddress(): {key, true},
				}),
				mkGetScript(map[string][]byte{
					p2shAddr.EncodeAddress(): redeemScript,
					wpProg.EncodeAddress():   witnessScript,
				}),
				nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// P2SH P2WSH Pay to pubkeyhash (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk := (*btcec.PublicKey)(&key.PublicKey).
				SerializeCompressed()
			pubKeyAddr, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk), &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}
			witnessScript, err := PayToAddrScript(pubKeyAddr)
			if err != nil {
				t.Errorf("failed to make redeemScript for %s: %v",
					msg, err)
				break
			}
			wsHash := sha256.Sum256(witnessScript)
			wpProg, err := btcutil.NewAddressWitnessScriptHash(wsHash[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			redeemScript, err := PayToAddrScript(wpProg)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}
			p2shAddr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			pkScript, err := PayToAddrScript(p2shAddr)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					pubKeyAddr.EncodeAddress(): {key, true},
				}),
				mkGetScript(map[string][]byte{
					p2shAddr.EncodeAddress(): redeemScript,
					wpProg.EncodeAddress():   witnessScript,
				}),
				nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Basic P2WSH Multisig
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			witnessScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			p2wsh := sha256.Sum256(witnessScript)
			scriptAddr, err := btcutil.NewAddressWitnessScriptHash(p2wsh[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): witnessScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Two part P2WSH multisig, sign with one key then the other.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			witnessScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			p2wsh := sha256.Sum256(witnessScript)
			scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
				p2wsh[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, witness, err := SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): witnessScript,
				}), nil, nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i], sigScript,
				pkScript, witness) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, witness, err = SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): witnessScript,
				}), sigScript, witness)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			err = checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i], sigScript,
				pkScript, witness)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Two part P2WSH multisig, sign with one key then both, check key dedup
	// correctly.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			witnessScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			p2wsh := sha256.Sum256(witnessScript)
			scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
				p2wsh[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v",
					msg, err)
				break
			}

			pkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make script pkscript for "+
					"%s: %v", msg, err)
				break
			}

			sigScript, witness, err := SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): witnessScript,
				}), nil, nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i],
				sigScript, pkScript, witness) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, witness, err = SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): witnessScript,
				}), sigScript, witness)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			// Now we should pass.
			err = checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i],
				sigScript, pkScript, witness)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Basic P2SH-P2WSH Multisig
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			witnessScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			p2wsh := sha256.Sum256(witnessScript)
			witAddr, err := btcutil.NewAddressWitnessScriptHash(p2wsh[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2WSH address for %s: %v",
					msg, err)
				break
			}

			redeemScript, err := PayToAddrScript(witAddr)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}
			p2shAddr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			pkScript, err := PayToAddrScript(p2shAddr)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			if err := signAndCheckWitness(msg, tx, sigHashes, i, inputAmounts[i],
				pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					p2shAddr.EncodeAddress(): redeemScript,
					witAddr.EncodeAddress():  witnessScript,
				}), nil); err != nil {
				t.Error(err)
				break
			}
		}
	}

	// Two part P2SH-P2WSH multisig, sign with one key then the other.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			witnessScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			p2wsh := sha256.Sum256(witnessScript)
			witAddr, err := btcutil.NewAddressWitnessScriptHash(p2wsh[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			redeemScript, err := PayToAddrScript(witAddr)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}
			scriptAddr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			pkScript, err := PayToAddrScript(scriptAddr)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, witness, err := SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): redeemScript,
					witAddr.EncodeAddress():    witnessScript,
				}), nil, nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i], sigScript,
				pkScript, witness) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, witness, err = SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.EncodeAddress(): redeemScript,
					witAddr.EncodeAddress():    witnessScript,
				}), sigScript, witness)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			err = checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i], sigScript,
				pkScript, witness)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Two part P2SH-P2WSH multisig, sign with one key then both, check key dedup
	// correctly.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			key1, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey for %s: %v",
					msg, err)
				break
			}

			pk1 := (*btcec.PublicKey)(&key1.PublicKey).
				SerializeCompressed()
			address1, err := btcutil.NewAddressPubKey(pk1,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			key2, err := btcec.NewPrivateKey(btcec.S256())
			if err != nil {
				t.Errorf("failed to make privKey 2 for %s: %v",
					msg, err)
				break
			}

			pk2 := (*btcec.PublicKey)(&key2.PublicKey).
				SerializeCompressed()
			address2, err := btcutil.NewAddressPubKey(pk2,
				&chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v",
					msg, err)
				break
			}

			witnessScript, err := MultiSigScript(
				[]*btcutil.AddressPubKey{address1, address2},
				2)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			p2wsh := sha256.Sum256(witnessScript)
			witAddr, err := btcutil.NewAddressWitnessScriptHash(p2wsh[:], &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}

			redeemScript, err := PayToAddrScript(witAddr)
			if err != nil {
				t.Errorf("failed to make P2SH address for %s: %v",
					msg, err)
				break
			}
			p2shAddr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.TestNet3Params)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			pkScript, err := PayToAddrScript(p2shAddr)
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			sigScript, witness, err := SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
				}), mkGetScript(map[string][]byte{
					p2shAddr.EncodeAddress(): redeemScript,
					witAddr.EncodeAddress():  witnessScript,
				}), nil, nil)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i],
				sigScript, pkScript, witness) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, witness, err = SignTxWitness(&chaincfg.TestNet3Params,
				tx, sigHashes, i, pkScript, inputAmounts[i], hashType,
				mkGetKey(map[string]addressToKey{
					address1.EncodeAddress(): {key1, true},
					address2.EncodeAddress(): {key2, true},
				}), mkGetScript(map[string][]byte{
					p2shAddr.EncodeAddress(): redeemScript,
					witAddr.EncodeAddress():  witnessScript,
				}), sigScript, witness)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			// Now we should pass.
			err = checkScriptsAndWitness(msg, tx, sigHashes, i, inputAmounts[i],
				sigScript, pkScript, witness)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}
}

func TestSignTxWitnessErrors(t *testing.T) {
	// make key
	// make script based on key.
	// sign with magic pixie dust.
	inputAmount := int64(5)
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0,
				},
				Sequence: 4294967295,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: 1,
			},
			{
				Value: 2,
			},
		},
		LockTime: 0,
	}

	sigHashes := NewTxSigHashes(tx)
	t.Run("test p2sh in p2sh is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeCompressed()

		p2pkhAddr, err := btcutil.NewAddressPubKey(pk1, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2pkh address %v", err)
		}
		p2pkhScript, err := PayToAddrScript(p2pkhAddr)
		if err != nil {
			t.Fatalf("failed to make p2pkh script %v", err)
		}
		p2shAddr, err := btcutil.NewAddressScriptHash(
			p2pkhScript, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make P2WPKH address for %v", err)
		}

		redeemScript, err := PayToAddrScript(p2shAddr)
		if err != nil {
			t.Fatalf("failed to make redeemScript %v", err)
		}
		scriptAddr, err := btcutil.NewAddressScriptHash(
			redeemScript, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make pkscript %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Errorf("failed to make pkscript %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkhAddr.EncodeAddress(): {key, true},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): redeemScript,
				p2shAddr.EncodeAddress():   p2pkhScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot nest P2SH inside P2SH" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test p2wpkh in p2wsh is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeCompressed()
		p2wpkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(
			btcutil.Hash160(pk1), &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make P2WPKH address for %v", err)
		}

		witnessScript, err := PayToAddrScript(p2wpkhAddr)
		if err != nil {
			t.Fatalf("failed to make witnessScript %v", err)
		}
		p2wsh := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make pkscript %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Errorf("failed to make pkscript %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2wpkhAddr.EncodeAddress(): {key, true},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot use P2WPKH inside P2WSH" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test p2sh in p2wsh is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeCompressed()

		p2pkhAddr, err := btcutil.NewAddressPubKey(pk1, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2pkh address %v", err)
		}
		p2pkhScript, err := PayToAddrScript(p2pkhAddr)
		if err != nil {
			t.Fatalf("failed to make p2pkh script %v", err)
		}
		p2shAddr, err := btcutil.NewAddressScriptHash(
			p2pkhScript, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make P2WPKH address for %v", err)
		}

		witnessScript, err := PayToAddrScript(p2shAddr)
		if err != nil {
			t.Fatalf("failed to make witnessScript %v", err)
		}
		p2wsh := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make pkscript %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Errorf("failed to make pkscript %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkhAddr.EncodeAddress(): {key, true},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
				p2shAddr.EncodeAddress():   p2pkhScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot nest P2SH or P2WSH inside P2WSH" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test p2wsh in p2wsh is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeCompressed()

		p2pkhAddr, err := btcutil.NewAddressPubKey(pk1, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2pkh address %v", err)
		}
		p2pkhScript, err := PayToAddrScript(p2pkhAddr)
		if err != nil {
			t.Fatalf("failed to make p2pkh script %v", err)
		}
		p2wsh1 := sha256.Sum256(p2pkhScript)
		p2wshAddr1, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh1[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make P2WPKH address for %v", err)
		}

		witnessScript, err := PayToAddrScript(p2wshAddr1)
		if err != nil {
			t.Fatalf("failed to make witnessScript %v", err)
		}
		p2wsh := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make pkscript %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Errorf("failed to make pkscript %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkhAddr.EncodeAddress(): {key, true},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
				p2wshAddr1.EncodeAddress(): p2pkhScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot nest P2SH or P2WSH inside P2WSH" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test uncompressed p2wpkh is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeUncompressed()

		p2wpkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(
			btcutil.Hash160(pk1), &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2wpkh address %v", err)
		}

		p2wpkhScript, err := PayToAddrScript(p2wpkhAddr)
		if err != nil {
			t.Fatalf("failed to make p2wpkh script %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, p2wpkhScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2wpkhAddr.EncodeAddress(): {key, false},
			}), mkGetScript(map[string][]byte{}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot use uncompressed keys with segwit" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test uncompressed p2wsh p2pkh is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeUncompressed()

		p2pkhAddr, err := btcutil.NewAddressPubKeyHash(
			btcutil.Hash160(pk1), &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make uncompressed p2pkh address %v", err)
		}

		witnessScript, err := PayToAddrScript(p2pkhAddr)
		if err != nil {
			t.Fatalf("failed to witness script %v", err)
		}

		p2wsh := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2wsh address %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Fatalf("failed to make p2wsh script %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkhAddr.EncodeAddress(): {key, false},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot use uncompressed keys with segwit" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test uncompressed p2wsh p2pk is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeUncompressed()

		p2pkAddr, err := btcutil.NewAddressPubKey(
			pk1, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make uncompressed p2pk address %v", err)
		}

		witnessScript, err := PayToAddrScript(p2pkAddr)
		if err != nil {
			t.Fatalf("failed to witness script %v", err)
		}

		p2wsh := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2wsh address %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Fatalf("failed to make p2wsh script %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkAddr.EncodeAddress(): {key, false},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot use uncompressed keys with segwit" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test uncompressed p2wsh multisig is forbidden", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeUncompressed()

		addr1, err := btcutil.NewAddressPubKey(pk1, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make address pubkey for multisig %v", err)
		}

		witnessScript, err := MultiSigScript([]*btcutil.AddressPubKey{addr1}, 1)
		if err != nil {
			t.Fatalf("failed to witness script %v", err)
		}

		p2wsh := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(
			p2wsh[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2wsh address %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Fatalf("failed to make p2wsh script %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				addr1.EncodeAddress(): {key, false},
			}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "cannot use uncompressed keys with segwit" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test unknown scriptPubKeys are rejected", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}
		prog := sha256.Sum256(key.Serialize())
		pkScript := make([]byte, 2)
		pkScript[0] = OP_1
		pkScript[1] = 32
		pkScript = append(pkScript, prog[:]...)

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{}), mkGetScript(map[string][]byte{}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "can't sign unknown transactions" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test unknown redeemScripts are rejected", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}
		prog := sha256.Sum256(key.Serialize())
		redeemScript := make([]byte, 2)
		redeemScript[0] = OP_1
		redeemScript[1] = 32
		redeemScript = append(redeemScript, prog[:]...)

		scriptAddr, err := btcutil.NewAddressScriptHash(redeemScript, &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make P2SH addr: %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Fatalf("failed to make P2SH script: %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): redeemScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "can't sign unknown transactions" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test unknown witnessScripts are rejected", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}
		prog := sha256.Sum256(key.Serialize())
		witnessScript := make([]byte, 2)
		witnessScript[0] = OP_1
		witnessScript[1] = 32
		witnessScript = append(witnessScript, prog[:]...)

		hash := sha256.Sum256(witnessScript)
		scriptAddr, err := btcutil.NewAddressWitnessScriptHash(hash[:], &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make P2SWH addr: %v", err)
		}

		pkScript, err := PayToAddrScript(scriptAddr)
		if err != nil {
			t.Fatalf("failed to make P2WSH script: %v", err)
		}

		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			tx, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{}), mkGetScript(map[string][]byte{
				scriptAddr.EncodeAddress(): witnessScript,
			}), nil, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "can't sign unknown transactions" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test scriptSigs are pushonly", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeCompressed()

		p2pkhAddr, err := btcutil.NewAddressPubKeyHash(
			btcutil.Hash160(pk1), &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2wpkh address %v", err)
		}

		pkScript, err := PayToAddrScript(p2pkhAddr)
		if err != nil {
			t.Fatalf("failed to make p2wpkh script %v", err)
		}

		txCpy := *tx
		prevScript := []byte{OP_RETURN}
		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			&txCpy, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkhAddr.EncodeAddress(): {key, true},
			}), mkGetScript(map[string][]byte{}), prevScript, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "signature script is not push only" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
	t.Run("test scriptSigs are valid scripts", func(t *testing.T) {
		key, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("failed to make privKey: %v", err)
		}

		pk1 := (*btcec.PublicKey)(&key.PublicKey).
			SerializeCompressed()

		p2pkhAddr, err := btcutil.NewAddressPubKeyHash(
			btcutil.Hash160(pk1), &chaincfg.TestNet3Params)
		if err != nil {
			t.Fatalf("failed to make p2wpkh address %v", err)
		}

		pkScript, err := PayToAddrScript(p2pkhAddr)
		if err != nil {
			t.Fatalf("failed to make p2wpkh script %v", err)
		}

		txCpy := *tx
		prevScript := []byte{0x20}
		_, _, err = SignTxWitness(&chaincfg.TestNet3Params,
			&txCpy, sigHashes, 0, pkScript, inputAmount, SigHashAll,
			mkGetKey(map[string]addressToKey{
				p2pkhAddr.EncodeAddress(): {key, true},
			}), mkGetScript(map[string][]byte{}), prevScript, nil)
		if err == nil {
			t.Fatal("Error expected")
		}
		if err.Error() != "opcode OP_DATA_32 requires 33 bytes, but script only has 1 remaining" {
			t.Fatalf("Unexpected error %v", err)
		}
	})
}

type tstInput struct {
	txout              *wire.TxOut
	sigscriptGenerates bool
	inputValidates     bool
	indexOutOfRange    bool
}

type tstSigScript struct {
	name               string
	inputs             []tstInput
	hashType           SigHashType
	compress           bool
	scriptAtWrongIndex bool
}

var coinbaseOutPoint = &wire.OutPoint{
	Index: (1 << 32) - 1,
}

// Pregenerated private key, with associated public key and pkScripts
// for the uncompressed and compressed hash160.
var (
	privKeyD = []byte{0x6b, 0x0f, 0xd8, 0xda, 0x54, 0x22, 0xd0, 0xb7,
		0xb4, 0xfc, 0x4e, 0x55, 0xd4, 0x88, 0x42, 0xb3, 0xa1, 0x65,
		0xac, 0x70, 0x7f, 0x3d, 0xa4, 0x39, 0x5e, 0xcb, 0x3b, 0xb0,
		0xd6, 0x0e, 0x06, 0x92}
	pubkeyX = []byte{0xb2, 0x52, 0xf0, 0x49, 0x85, 0x78, 0x03, 0x03, 0xc8,
		0x7d, 0xce, 0x51, 0x7f, 0xa8, 0x69, 0x0b, 0x91, 0x95, 0xf4,
		0xf3, 0x5c, 0x26, 0x73, 0x05, 0x05, 0xa2, 0xee, 0xbc, 0x09,
		0x38, 0x34, 0x3a}
	pubkeyY = []byte{0xb7, 0xc6, 0x7d, 0xb2, 0xe1, 0xff, 0xc8, 0x43, 0x1f,
		0x63, 0x32, 0x62, 0xaa, 0x60, 0xc6, 0x83, 0x30, 0xbd, 0x24,
		0x7e, 0xef, 0xdb, 0x6f, 0x2e, 0x8d, 0x56, 0xf0, 0x3c, 0x9f,
		0x6d, 0xb6, 0xf8}
	uncompressedPkScript = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x86, 0xc9, 0xfa, 0x88, 0xac}
	compressedPkScript = []byte{0x76, 0xa9, 0x14, 0x27, 0x4d, 0x9f, 0x7f,
		0x61, 0x7e, 0x7c, 0x7a, 0x1c, 0x1f, 0xb2, 0x75, 0x79, 0x10,
		0x43, 0x65, 0x68, 0x27, 0x9d, 0x86, 0x88, 0xac}
	shortPkScript = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x88, 0xac}
	uncompressedAddrStr = "1L6fd93zGmtzkK6CsZFVVoCwzZV3MUtJ4F"
	compressedAddrStr   = "14apLppt9zTq6cNw8SDfiJhk9PhkZrQtYZ"
)

// Pretend output amounts.
const coinbaseVal = 2500000000
const fee = 5000000

var sigScriptTests = []tstSigScript{
	{
		name: "one input uncompressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs uncompressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "one input compressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs compressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashNone",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashNone,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashSingle",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashSingle,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashAnyoneCanPay",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAnyOneCanPay,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType non-standard",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           0x04,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "invalid compression",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     false,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "short PkScript",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, shortPkScript),
				sigscriptGenerates: false,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "valid script at wrong index",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
	{
		name: "index out of range",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
}

// Test the sigscript generation for valid and invalid inputs, all
// hashTypes, and with and without compression.  This test creates
// sigscripts to spend fake coinbase inputs, as sigscripts cannot be
// created for the MsgTxs in txTests, since they come from the blockchain
// and we don't have the private keys.
func TestSignatureScript(t *testing.T) {
	t.Parallel()

	privKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), privKeyD)

nexttest:
	for i := range sigScriptTests {
		tx := wire.NewMsgTx(wire.TxVersion)

		output := wire.NewTxOut(500, []byte{OP_RETURN})
		tx.AddTxOut(output)

		for range sigScriptTests[i].inputs {
			txin := wire.NewTxIn(coinbaseOutPoint, nil, nil)
			tx.AddTxIn(txin)
		}

		var script []byte
		var err error
		for j := range tx.TxIn {
			var idx int
			if sigScriptTests[i].inputs[j].indexOutOfRange {
				t.Errorf("at test %v", sigScriptTests[i].name)
				idx = len(sigScriptTests[i].inputs)
			} else {
				idx = j
			}
			script, err = SignatureScript(tx, idx,
				sigScriptTests[i].inputs[j].txout.PkScript,
				sigScriptTests[i].hashType, privKey,
				sigScriptTests[i].compress)

			if (err == nil) != sigScriptTests[i].inputs[j].sigscriptGenerates {
				if err == nil {
					t.Errorf("passed test '%v' incorrectly",
						sigScriptTests[i].name)
				} else {
					t.Errorf("failed test '%v': %v",
						sigScriptTests[i].name, err)
				}
				continue nexttest
			}
			if !sigScriptTests[i].inputs[j].sigscriptGenerates {
				// done with this test
				continue nexttest
			}

			tx.TxIn[j].SignatureScript = script
		}

		// If testing using a correct sigscript but for an incorrect
		// index, use last input script for first input.  Requires > 0
		// inputs for test.
		if sigScriptTests[i].scriptAtWrongIndex {
			tx.TxIn[0].SignatureScript = script
			sigScriptTests[i].inputs[0].inputValidates = false
		}

		// Validate tx input scripts
		scriptFlags := ScriptBip16 | ScriptVerifyDERSignatures
		for j := range tx.TxIn {
			vm, err := NewEngine(sigScriptTests[i].
				inputs[j].txout.PkScript, tx, j, scriptFlags, nil, nil, 0)
			if err != nil {
				t.Errorf("cannot create script vm for test %v: %v",
					sigScriptTests[i].name, err)
				continue nexttest
			}
			err = vm.Execute()
			if (err == nil) != sigScriptTests[i].inputs[j].inputValidates {
				if err == nil {
					t.Errorf("passed test '%v' validation incorrectly: %v",
						sigScriptTests[i].name, err)
				} else {
					t.Errorf("failed test '%v' validation: %v",
						sigScriptTests[i].name, err)
				}
				continue nexttest
			}
		}
	}
}

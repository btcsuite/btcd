package txscript

import (
	"fmt"

	"github.com/btcsuite/btcd/wire"
)

// VerifyTaprootKeySpend attempts to verify a top-level taproot key spend,
// returning a non-nil error if the passed signature is invalid.  If a sigCache
// is passed in, then the sig cache will be consulted to skip full verification
// of a signature that has already been seen. Witness program here should be
// the 32-byte x-only schnorr output public key.
//
// NOTE: The TxSigHashes MUST be passed in and fully populated.
func VerifyTaprootKeySpend(witnessProgram []byte, rawSig []byte, tx *wire.MsgTx,
	inputIndex int, prevOuts PrevOutputFetcher, hashCache *TxSigHashes,
	sigCache *SigCache) error {

	// First, we'll need to extract the public key from the witness
	// program.
	rawKey := witnessProgram

	// Extract the annex if it exists, so we can compute the proper proper
	// sighash below.
	var annex []byte
	witness := tx.TxIn[inputIndex].Witness
	if isAnnexedWitness(witness) {
		annex, _ = extractAnnex(witness)
	}

	// Now that we have the public key, we can create a new top-level
	// keyspend verifier that'll handle all the sighash and schnorr
	// specifics for us.
	keySpendVerifier, err := newTaprootSigVerifier(
		rawKey, rawSig, tx, inputIndex, prevOuts, sigCache,
		hashCache, annex,
	)
	if err != nil {
		return err
	}

	valid := keySpendVerifier.Verify()
	if valid {
		return nil
	}

	// TODO(roasbeef): add proper error
	return fmt.Errorf("invalid sig")
}

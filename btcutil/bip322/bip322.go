package bip322

import (
	"github.com/btcsuite/btcd/txscript"
)

// bip322VerifyFlags are the script verification flags required by BIP-322.
const bip322VerifyFlags = txscript.ScriptBip16 |
	txscript.ScriptVerifyWitness |
	txscript.ScriptVerifyTaproot |
	txscript.ScriptVerifyCleanStack |
	txscript.ScriptVerifyDERSignatures |
	txscript.ScriptVerifyLowS |
	txscript.ScriptVerifyMinimalData |
	txscript.ScriptVerifyNullFail |
	txscript.ScriptVerifyStrictEncoding |
	txscript.ScriptVerifyMinimalIf |
	txscript.ScriptVerifyWitnessPubKeyType |
	txscript.ScriptVerifyConstScriptCode

// bip322MsgTag is the BIP-322 tagged hash message tag as defined in BIP-322.
// See https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
var bip322MsgTag = []byte("BIP0322-signed-message")

package txscript

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

const (
	// MaxClaimScriptSize is the max claim script size in bytes, not including the script pubkey part of the script.
	MaxClaimScriptSize = 8192

	// MaxClaimNameSize is the max claim name size in bytes, for all claim trie transactions.
	MaxClaimNameSize = 255

	ClaimIDLength = 160 / 8

	claimScriptVersion = 0
)

// These constants are used to identify a specific claim script Error.
// The error code starts from 200, which leaves enough room between the rest
// of script error codes (numErrorCodes)
const (
	// ErrNotClaimScript is returned when the script does not have a ClaimScript Opcode.
	ErrNotClaimScript ErrorCode = iota + 200

	// ErrInvalidClaimNameScript is returned a claim name script does not conform to the format.
	ErrInvalidClaimNameScript

	// ErrInvalidClaimSupportScript is returned a claim support script does not conform to the format.
	ErrInvalidClaimSupportScript

	// ErrInvalidClaimUpdateScript is returned a claim update script does not conform to the format.
	ErrInvalidClaimUpdateScript
)

func claimScriptError(c ErrorCode, desc string) Error {
	return Error{ErrorCode: c, Description: desc}
}

// ClaimNameScript creates a claim name script.
func ClaimNameScript(name string, value string) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_CLAIMNAME).AddData([]byte(name)).AddData([]byte(value)).
		AddOp(OP_2DROP).AddOp(OP_DROP).AddOp(OP_TRUE).Script()
}

// ClaimSupportScript creates a support claim script.
func ClaimSupportScript(name string, claimID []byte, value []byte) ([]byte, error) {
	builder := NewScriptBuilder().AddOp(OP_SUPPORTCLAIM).AddData([]byte(name)).AddData(claimID)
	if len(value) > 0 {
		return builder.addData(value).AddOp(OP_2DROP).AddOp(OP_2DROP).AddOp(OP_TRUE).Script()
	}
	return builder.AddOp(OP_2DROP).AddOp(OP_DROP).AddOp(OP_TRUE).Script()
}

// ClaimUpdateScript creates an update claim script.
func ClaimUpdateScript(name string, claimID []byte, value string) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_UPDATECLAIM).AddData([]byte(name)).AddData(claimID).AddData([]byte(value)).
		AddOp(OP_2DROP).AddOp(OP_2DROP).AddOp(OP_TRUE).Script()
}

// ClaimScript represents of one of the ClaimNameScript, ClaimSupportScript, and ClaimUpdateScript.
type ClaimScript struct {
	Opcode  byte
	Name    []byte
	ClaimID []byte
	Value   []byte
	Size    int
}

// ExtractClaimScript exctracts the claim script from the script if it has one.
// The returned ClaimScript is invalidated if the given script is modified.
func ExtractClaimScript(script []byte) (*ClaimScript, error) {

	var cs ClaimScript

	tokenizer := MakeScriptTokenizer(claimScriptVersion, script)
	if !tokenizer.Next() {
		return nil, claimScriptError(ErrNotClaimScript, "not a claim script opcode")
	}

	cs.Opcode = tokenizer.Opcode()

	switch tokenizer.Opcode() {
	case OP_CLAIMNAME:
		// OP_CLAIMNAME <Name> <Value> OP_2DROP OP_DROP <P2PKH>
		if !tokenizer.Next() || len(tokenizer.Data()) > MaxClaimNameSize {
			str := fmt.Sprintf("name size %d exceeds limit %d", len(tokenizer.data), MaxClaimNameSize)
			return nil, claimScriptError(ErrInvalidClaimNameScript, str)
		}
		cs.Name = tokenizer.data

		if !tokenizer.Next() {
			return nil, claimScriptError(ErrInvalidClaimNameScript, "expect value")
		}
		cs.Value = tokenizer.Data()

		if !tokenizer.Next() || tokenizer.Opcode() != OP_2DROP ||
			!tokenizer.Next() || tokenizer.Opcode() != OP_DROP {
			str := fmt.Sprintf("expect OP_2DROP OP_DROP")
			return nil, claimScriptError(ErrInvalidClaimNameScript, str)
		}

		cs.Size = int(tokenizer.ByteIndex())
		return &cs, nil

	case OP_SUPPORTCLAIM:
		// OP_SUPPORTCLAIM <Name> <ClaimID>         OP_2DROP OP_DROP <P2PKH>
		// OP_SUPPORTCLAIM <Name> <ClaimID> <Value> OP_2DROP OP_2DROP <P2PKH>
		if !tokenizer.Next() || len(tokenizer.Data()) > MaxClaimNameSize {
			str := fmt.Sprintf("name size %d exceeds limit %d", len(tokenizer.data), MaxClaimNameSize)
			return nil, claimScriptError(ErrInvalidClaimSupportScript, str)
		}
		cs.Name = tokenizer.data

		if !tokenizer.Next() || len(tokenizer.Data()) != ClaimIDLength {
			str := fmt.Sprintf("expect claim id length %d, instead of %d", ClaimIDLength, len(tokenizer.data))
			return nil, claimScriptError(ErrInvalidClaimSupportScript, str)
		}
		cs.ClaimID = tokenizer.Data()

		if !tokenizer.Next() {
			return nil, claimScriptError(ErrInvalidClaimSupportScript, "incomplete script")
		}

		switch {
		case tokenizer.Opcode() == OP_2DROP:
			// Case 1: OP_SUPPORTCLAIM <Name> <ClaimID> OP_2DROP OP_DROP <P2PKH>
			if !tokenizer.Next() || tokenizer.Opcode() != OP_DROP {
				str := fmt.Sprintf("expect OP_2DROP OP_DROP")
				return nil, claimScriptError(ErrInvalidClaimSupportScript, str)
			}

		case len(tokenizer.Data()) != 0:
			// Case 2: OP_SUPPORTCLAIM <Name> <ClaimID> <Dummy Value> OP_2DROP OP_2DROP <P2PKH>
			//         (old bug: non-length size dummy value?)
			cs.Value = tokenizer.Data()
			if !tokenizer.Next() || tokenizer.Opcode() != OP_2DROP ||
				!tokenizer.Next() || tokenizer.Opcode() != OP_2DROP {
				str := fmt.Sprintf("expect OP_2DROP OP_2DROP")
				return nil, claimScriptError(ErrInvalidClaimSupportScript, str)
			}
		default:
			str := fmt.Sprintf("expect OP_2DROP OP_DROP")
			return nil, claimScriptError(ErrInvalidClaimSupportScript, str)
		}

		cs.Size = int(tokenizer.ByteIndex())
		return &cs, nil

	case OP_UPDATECLAIM:

		// OP_UPDATECLAIM <Name> <ClaimID> <Value> OP_2DROP OP_2DROP <P2PKH>
		if !tokenizer.Next() || len(tokenizer.Data()) > MaxClaimNameSize {
			str := fmt.Sprintf("name size %d exceeds limit %d", len(tokenizer.data), MaxClaimNameSize)
			return nil, claimScriptError(ErrInvalidClaimUpdateScript, str)
		}
		cs.Name = tokenizer.data

		if !tokenizer.Next() || len(tokenizer.Data()) != ClaimIDLength {
			str := fmt.Sprintf("expect claim id length %d, instead of %d", ClaimIDLength, len(tokenizer.data))
			return nil, claimScriptError(ErrInvalidClaimUpdateScript, str)
		}
		cs.ClaimID = tokenizer.Data()

		if !tokenizer.Next() {
			str := fmt.Sprintf("expect value")
			return nil, claimScriptError(ErrInvalidClaimUpdateScript, str)
		}
		cs.Value = tokenizer.Data()

		if !tokenizer.Next() || tokenizer.Opcode() != OP_2DROP ||
			!tokenizer.Next() || tokenizer.Opcode() != OP_2DROP {
			str := fmt.Sprintf("expect OP_2DROP OP_2DROP")
			return nil, claimScriptError(ErrInvalidClaimUpdateScript, str)
		}

		cs.Size = int(tokenizer.ByteIndex())
		return &cs, nil

	default:
		return nil, claimScriptError(ErrNotClaimScript, "")
	}
}

// StripClaimScriptPrefix strips prefixed claim script, if any.
func StripClaimScriptPrefix(script []byte) []byte {
	cs, err := ExtractClaimScript(script)
	if err != nil {
		return script
	}
	return script[cs.Size:]
}

const illegalChars = "=&#:*$%?/;\\\b\n\t\r\x00"

func AllClaimsAreSane(script []byte, enforceSoftFork bool) error {
	cs, err := ExtractClaimScript(script)
	if IsErrorCode(err, ErrNotClaimScript) {
		return nil
	}
	if err != nil {
		return err
	}
	if enforceSoftFork {
		if !utf8.Valid(cs.Name) {
			return fmt.Errorf("claim name is not valid UTF-8")
		}
		if bytes.ContainsAny(cs.Name, illegalChars) {
			return fmt.Errorf("claim name has illegal chars; it should not contain any of these: %s", illegalChars)
		}
	}

	return nil
}

package txscript

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreationParseLoopClaim(t *testing.T) {

	r := require.New(t)

	// OP_CLAIMNAME <Name> <Value> OP_2DROP OP_DROP <P2PKH>
	script, err := ClaimNameScript("tester", "value")
	r.NoError(err)
	cs, err := ExtractClaimScript(script)
	r.NoError(err)
	r.Equal(byte(OP_CLAIMNAME), cs.Opcode)
	r.Equal([]byte("tester"), cs.Name)
	r.Equal([]byte("value"), cs.Value)
}

func TestCreationParseLoopUpdate(t *testing.T) {

	r := require.New(t)

	claimID := []byte("12345123451234512345")
	claim, err := ClaimUpdateScript("tester", claimID, "value")
	r.NoError(err)
	cs, err := ExtractClaimScript(claim)
	r.NoError(err)
	r.Equal(byte(OP_UPDATECLAIM), cs.Opcode)
	r.Equal([]byte("tester"), cs.Name)
	r.Equal(claimID, cs.ClaimID)
	r.Equal([]byte("value"), cs.Value)
}

func TestCreationParseLoopSupport(t *testing.T) {

	r := require.New(t)

	claimID := []byte("12345123451234512345")

	// case 1: OP_SUPPORTCLAIM <Name> <ClaimID> OP_2DROP OP_DROP <P2PKH>
	script, err := ClaimSupportScript("tester", claimID, nil)
	r.NoError(err)
	cs, err := ExtractClaimScript(script)
	r.NoError(err)

	r.Equal(byte(OP_SUPPORTCLAIM), cs.Opcode)
	r.Equal([]byte("tester"), cs.Name)
	r.Equal(claimID, cs.ClaimID)
	r.Nil(cs.Value)

	// case 2: OP_SUPPORTCLAIM <Name> <ClaimID> <Value> OP_2DROP OP_2DROP <P2PKH>
	script, err = ClaimSupportScript("tester", claimID, []byte("value"))
	r.NoError(err)
	cs, err = ExtractClaimScript(script)
	r.NoError(err)

	r.Equal(byte(OP_SUPPORTCLAIM), cs.Opcode)
	r.Equal([]byte("tester"), cs.Name)
	r.Equal(claimID, cs.ClaimID)
	r.Equal([]byte("value"), cs.Value)

}

func TestInvalidChars(t *testing.T) {
	r := require.New(t)

	script, err := ClaimNameScript("tester", "value")
	r.NoError(err)
	r.NoError(AllClaimsAreSane(script, true))

	for i := range []byte(illegalChars) {
		script, err := ClaimNameScript("a"+illegalChars[i:i+1], "value")
		r.NoError(err)
		r.Error(AllClaimsAreSane(script, true))
	}
}

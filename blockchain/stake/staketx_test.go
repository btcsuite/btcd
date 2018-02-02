// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake_test

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// SSTX TESTING -------------------------------------------------------------------

// TestSStx ensures the CheckSStx and IsSStx functions correctly recognize stake
// submission transactions.
func TestSStx(t *testing.T) {
	var sstx = dcrutil.NewTx(sstxMsgTx)
	sstx.SetTree(wire.TxTreeStake)
	sstx.SetIndex(0)

	err := stake.CheckSStx(sstx.MsgTx())
	if err != nil {
		t.Errorf("CheckSStx: unexpected err: %v", err)
	}
	if !stake.IsSStx(sstx.MsgTx()) {
		t.Errorf("IsSStx claimed a valid sstx is invalid")
	}

	// ---------------------------------------------------------------------------
	// Test for an OP_RETURN commitment push of the maximum size
	biggestPush := []byte{
		0x6a, 0x4b, // OP_RETURN Push 75-bytes
		0x14, 0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, // 75 bytes
		0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde,
	}

	sstx = dcrutil.NewTxDeep(sstxMsgTx)
	sstx.MsgTx().TxOut[1].PkScript = biggestPush
	sstx.SetTree(wire.TxTreeStake)
	sstx.SetIndex(0)

	err = stake.CheckSStx(sstx.MsgTx())
	if err != nil {
		t.Errorf("CheckSStx: unexpected err: %v", err)
	}
	if !stake.IsSStx(sstx.MsgTx()) {
		t.Errorf("IsSStx claimed a valid sstx is invalid")
	}
}

// TestSSTxErrors ensures the CheckSStx and IsSStx functions correctly identify
// errors in stake submission transactions and does not report them as valid.
func TestSSTxErrors(t *testing.T) {
	// Initialize the buffer for later manipulation
	var buf bytes.Buffer
	buf.Grow(sstxMsgTx.SerializeSize())
	err := sstxMsgTx.Serialize(&buf)
	if err != nil {
		t.Errorf("Error serializing the reference sstx: %v", err)
	}
	bufBytes := buf.Bytes()

	// ---------------------------------------------------------------------------
	// Test too many inputs with sstxMsgTxExtraInputs

	var sstxExtraInputs = dcrutil.NewTx(sstxMsgTxExtraInput)
	sstxExtraInputs.SetTree(wire.TxTreeStake)
	sstxExtraInputs.SetIndex(0)

	err = stake.CheckSStx(sstxExtraInputs.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxTooManyInputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxTooManyInputs, err)
	}
	if stake.IsSStx(sstxExtraInputs.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test too many outputs with sstxMsgTxExtraOutputs

	var sstxExtraOutputs = dcrutil.NewTx(sstxMsgTxExtraOutputs)
	sstxExtraOutputs.SetTree(wire.TxTreeStake)
	sstxExtraOutputs.SetIndex(0)

	err = stake.CheckSStx(sstxExtraOutputs.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxTooManyOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxTooManyOutputs, err)
	}
	if stake.IsSStx(sstxExtraOutputs.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Check to make sure the first output is OP_SSTX tagged

	var tx wire.MsgTx
	testFirstOutTagged := bytes.Replace(bufBytes,
		[]byte{0x00, 0xe3, 0x23, 0x21, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x1a, 0xba},
		[]byte{0x00, 0xe3, 0x23, 0x21, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x19},
		1)

	// Deserialize the manipulated tx
	rbuf := bytes.NewReader(testFirstOutTagged)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var sstxUntaggedOut = dcrutil.NewTx(&tx)
	sstxUntaggedOut.SetTree(wire.TxTreeStake)
	sstxUntaggedOut.SetIndex(0)

	err = stake.CheckSStx(sstxUntaggedOut.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInvalidOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInvalidOutputs, err)
	}
	if stake.IsSStx(sstxUntaggedOut.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for mismatched number of inputs versus number of outputs

	var sstxInsOutsMismatched = dcrutil.NewTx(sstxMismatchedInsOuts)
	sstxInsOutsMismatched.SetTree(wire.TxTreeStake)
	sstxInsOutsMismatched.SetIndex(0)

	err = stake.CheckSStx(sstxInsOutsMismatched.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInOutProportions {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInOutProportions, err)
	}
	if stake.IsSStx(sstxInsOutsMismatched.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for bad version of output.
	var sstxBadVerOut = dcrutil.NewTx(sstxBadVersionOut)
	sstxBadVerOut.SetTree(wire.TxTreeStake)
	sstxBadVerOut.SetIndex(0)

	err = stake.CheckSStx(sstxBadVerOut.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInvalidOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInvalidOutputs, err)
	}
	if stake.IsSStx(sstxBadVerOut.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for second or more output not being OP_RETURN push

	var sstxNoNullData = dcrutil.NewTx(sstxNullDataMissing)
	sstxNoNullData.SetTree(wire.TxTreeStake)
	sstxNoNullData.SetIndex(0)

	err = stake.CheckSStx(sstxNoNullData.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInvalidOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInvalidOutputs, err)
	}
	if stake.IsSStx(sstxNoNullData.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for change output being in the wrong place

	var sstxNullDataMis = dcrutil.NewTx(sstxNullDataMisplaced)
	sstxNullDataMis.SetTree(wire.TxTreeStake)
	sstxNullDataMis.SetIndex(0)

	err = stake.CheckSStx(sstxNullDataMis.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInvalidOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInvalidOutputs, err)
	}
	if stake.IsSStx(sstxNullDataMis.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for too short of a pubkeyhash being given in an OP_RETURN output

	testPKHLength := bytes.Replace(bufBytes,
		[]byte{
			0x20, 0x6a, 0x1e, 0x94, 0x8c, 0x76, 0x5a, 0x69,
			0x14, 0xd4, 0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda,
			0x2c, 0x2f, 0x6b, 0x52, 0xde, 0x3d, 0x7c,
		},
		[]byte{
			0x1f, 0x6a, 0x1d, 0x94, 0x8c, 0x76, 0x5a, 0x69,
			0x14, 0xd4, 0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda,
			0x2c, 0x2f, 0x6b, 0x52, 0xde, 0x3d,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testPKHLength)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var sstxWrongPKHLength = dcrutil.NewTx(&tx)
	sstxWrongPKHLength.SetTree(wire.TxTreeStake)
	sstxWrongPKHLength.SetIndex(0)

	err = stake.CheckSStx(sstxWrongPKHLength.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInvalidOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInvalidOutputs, err)
	}
	if stake.IsSStx(sstxWrongPKHLength.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an invalid OP_RETURN prefix with too big of a push
	tooBigPush := []byte{
		0x6a, 0x4c, 0x4c, // OP_RETURN Push 76-bytes
		0x14, 0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, // 76 bytes
		0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d,
	}

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(bufBytes)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}
	tx.TxOut[1].PkScript = tooBigPush

	var sstxWrongPrefix = dcrutil.NewTx(&tx)
	sstxWrongPrefix.SetTree(wire.TxTreeStake)
	sstxWrongPrefix.SetIndex(0)

	err = stake.CheckSStx(sstxWrongPrefix.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSStxInvalidOutputs {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			stake.ErrSStxInvalidOutputs, err)
	}
	if stake.IsSStx(sstxWrongPrefix.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}
}

// SSGEN TESTING ------------------------------------------------------------------

// TestSSGen ensures the CheckSSGen and IsSSGen functions correctly recognize
// stake submission generation transactions.
func TestSSGen(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	err := stake.CheckSSGen(ssgen.MsgTx())
	if err != nil {
		t.Errorf("IsSSGen: unexpected err: %v", err)
	}
	if !stake.IsSSGen(ssgen.MsgTx()) {
		t.Errorf("IsSSGen claimed a valid ssgen is invalid")
	}

	// Test for an OP_RETURN VoteBits push of the maximum size
	biggestPush := []byte{
		0x6a, 0x4b, // OP_RETURN Push 75-bytes
		0x14, 0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, // 75 bytes
		0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde,
	}

	ssgen = dcrutil.NewTxDeep(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)
	ssgen.MsgTx().TxOut[1].PkScript = biggestPush

	err = stake.CheckSSGen(ssgen.MsgTx())
	if err != nil {
		t.Errorf("IsSSGen: unexpected err: %v", err)
	}
	if !stake.IsSSGen(ssgen.MsgTx()) {
		t.Errorf("IsSSGen claimed a valid ssgen is invalid")
	}

}

// TestSSGenErrors ensures the CheckSSGen and IsSSGen functions correctly
// identify errors in stake submission generation transactions and does not
// report them as valid.
func TestSSGenErrors(t *testing.T) {
	// Initialize the buffer for later manipulation
	var buf bytes.Buffer
	buf.Grow(ssgenMsgTx.SerializeSize())
	err := ssgenMsgTx.Serialize(&buf)
	if err != nil {
		t.Errorf("Error serializing the reference sstx: %v", err)
	}
	bufBytes := buf.Bytes()

	// ---------------------------------------------------------------------------
	// Test too many inputs with ssgenMsgTxExtraInputs

	var ssgenExtraInputs = dcrutil.NewTx(ssgenMsgTxExtraInput)
	ssgenExtraInputs.SetTree(wire.TxTreeStake)
	ssgenExtraInputs.SetIndex(0)

	err = stake.CheckSSGen(ssgenExtraInputs.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenWrongNumInputs {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenWrongNumInputs, err)
	}
	if stake.IsSSGen(ssgenExtraInputs.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test too many outputs with sstxMsgTxExtraOutputs

	var ssgenExtraOutputs = dcrutil.NewTx(ssgenMsgTxExtraOutputs)
	ssgenExtraOutputs.SetTree(wire.TxTreeStake)
	ssgenExtraOutputs.SetIndex(0)

	err = stake.CheckSSGen(ssgenExtraOutputs.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenTooManyOutputs {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenTooManyOutputs, err)
	}
	if stake.IsSSGen(ssgenExtraOutputs.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test 0th input not being stakebase error

	var ssgenStakeBaseWrong = dcrutil.NewTx(ssgenMsgTxStakeBaseWrong)
	ssgenStakeBaseWrong.SetTree(wire.TxTreeStake)
	ssgenStakeBaseWrong.SetIndex(0)

	err = stake.CheckSSGen(ssgenStakeBaseWrong.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenNoStakebase {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenNoStakebase, err)
	}
	if stake.IsSSGen(ssgenStakeBaseWrong.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Wrong tree for inputs test

	// Replace TxTreeStake with TxTreeRegular
	testWrongTreeInputs := bytes.Replace(bufBytes,
		[]byte{0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87, 0x00,
			0x00, 0x00, 0x00, 0x01},
		[]byte{0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87, 0x00,
			0x00, 0x00, 0x00, 0x00},
		1)

	// Deserialize the manipulated tx
	var tx wire.MsgTx
	rbuf := bytes.NewReader(testWrongTreeInputs)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongTreeIns = dcrutil.NewTx(&tx)
	ssgenWrongTreeIns.SetTree(wire.TxTreeStake)
	ssgenWrongTreeIns.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongTreeIns.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenWrongTxTree {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenWrongTxTree, err)
	}
	if stake.IsSSGen(ssgenWrongTreeIns.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for bad version of output.
	var ssgenTxBadVerOut = dcrutil.NewTx(ssgenMsgTxBadVerOut)
	ssgenTxBadVerOut.SetTree(wire.TxTreeStake)
	ssgenTxBadVerOut.SetIndex(0)

	err = stake.CheckSSGen(ssgenTxBadVerOut.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenBadGenOuts {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenBadGenOuts, err)
	}
	if stake.IsSSGen(ssgenTxBadVerOut.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test 0th output not being OP_RETURN push

	var ssgenWrongZeroethOut = dcrutil.NewTx(ssgenMsgTxWrongZeroethOut)
	ssgenWrongZeroethOut.SetTree(wire.TxTreeStake)
	ssgenWrongZeroethOut.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongZeroethOut.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenNoReference {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenNoReference, err)
	}
	if stake.IsSSGen(ssgenWrongZeroethOut.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for too short of an OP_RETURN push being given in the 0th tx out

	testDataPush0Length := bytes.Replace(bufBytes,
		[]byte{
			0x26, 0x6a, 0x24,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23, 0x21,
		},
		[]byte{
			0x25, 0x6a, 0x23,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testDataPush0Length)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongDataPush0Length = dcrutil.NewTx(&tx)
	ssgenWrongDataPush0Length.SetTree(wire.TxTreeStake)
	ssgenWrongDataPush0Length.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongDataPush0Length.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenBadReference {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenBadReference, err)
	}
	if stake.IsSSGen(ssgenWrongDataPush0Length.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an invalid OP_RETURN prefix

	testNullData0Prefix := bytes.Replace(bufBytes,
		[]byte{
			0x26, 0x6a, 0x24,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23, 0x21,
		},
		[]byte{ // This uses an OP_PUSHDATA_1 35-byte push to achieve 36 bytes
			0x26, 0x6a, 0x4c, 0x23,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testNullData0Prefix)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongNullData0Prefix = dcrutil.NewTx(&tx)
	ssgenWrongNullData0Prefix.SetTree(wire.TxTreeStake)
	ssgenWrongNullData0Prefix.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongNullData0Prefix.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenBadReference {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenBadReference, err)
	}
	if stake.IsSSGen(ssgenWrongNullData0Prefix.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test 1st output not being OP_RETURN push

	var ssgenWrongFirstOut = dcrutil.NewTx(ssgenMsgTxWrongFirstOut)
	ssgenWrongFirstOut.SetTree(wire.TxTreeStake)
	ssgenWrongFirstOut.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongFirstOut.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenNoVotePush {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenNoVotePush, err)
	}
	if stake.IsSSGen(ssgenWrongFirstOut.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for too short of an OP_RETURN push being given in the 1st tx out
	testDataPush1Length := bytes.Replace(bufBytes,
		[]byte{
			0x04, 0x6a, 0x02, 0x94, 0x8c,
		},
		[]byte{
			0x03, 0x6a, 0x01, 0x94,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testDataPush1Length)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongDataPush1Length = dcrutil.NewTx(&tx)
	ssgenWrongDataPush1Length.SetTree(wire.TxTreeStake)
	ssgenWrongDataPush1Length.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongDataPush1Length.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenBadVotePush {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenBadVotePush, err)
	}
	if stake.IsSSGen(ssgenWrongDataPush1Length.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an invalid OP_RETURN prefix

	testNullData1Prefix := bytes.Replace(bufBytes,
		[]byte{
			0x04, 0x6a, 0x02, 0x94, 0x8c,
		},
		[]byte{ // This uses an OP_PUSHDATA_1 2-byte push to do the push in 5 bytes
			0x05, 0x6a, 0x4c, 0x02, 0x00, 0x00,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testNullData1Prefix)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongNullData1Prefix = dcrutil.NewTx(&tx)
	ssgenWrongNullData1Prefix.SetTree(wire.TxTreeStake)
	ssgenWrongNullData1Prefix.SetIndex(0)

	err = stake.CheckSSGen(ssgenWrongNullData1Prefix.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenBadVotePush {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenBadVotePush, err)
	}
	if stake.IsSSGen(ssgenWrongNullData1Prefix.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an index 2+ output being not OP_SSGEN tagged

	testGenOutputUntagged := bytes.Replace(bufBytes,
		[]byte{
			0x1a, 0xbb, 0x76, 0xa9, 0x14, 0xc3, 0x98,
		},
		[]byte{
			0x19, 0x76, 0xa9, 0x14, 0xc3, 0x98,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testGenOutputUntagged)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgentestGenOutputUntagged = dcrutil.NewTx(&tx)
	ssgentestGenOutputUntagged.SetTree(wire.TxTreeStake)
	ssgentestGenOutputUntagged.SetIndex(0)

	err = stake.CheckSSGen(ssgentestGenOutputUntagged.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSGenBadGenOuts {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			stake.ErrSSGenBadGenOuts, err)
	}
	if stake.IsSSGen(ssgentestGenOutputUntagged.MsgTx()) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}
}

// SSRTX TESTING ------------------------------------------------------------------

// TestSSRtx ensures the CheckSSRtx and IsSSRtx functions correctly recognize
// stake submission revocation transactions.
func TestSSRtx(t *testing.T) {
	var ssrtx = dcrutil.NewTx(ssrtxMsgTx)
	ssrtx.SetTree(wire.TxTreeStake)
	ssrtx.SetIndex(0)

	err := stake.CheckSSRtx(ssrtx.MsgTx())
	if err != nil {
		t.Errorf("IsSSRtx: unexpected err: %v", err)
	}
	if !stake.IsSSRtx(ssrtx.MsgTx()) {
		t.Errorf("IsSSRtx claimed a valid ssrtx is invalid")
	}
}

// TestSSRtxErrors ensures the CheckSSRtx and IsSSRtx functions correctly
// identify errors in stake submission revocation transactions and does not
// report them as valid.
func TestIsSSRtxErrors(t *testing.T) {
	// Initialize the buffer for later manipulation
	var buf bytes.Buffer
	buf.Grow(ssrtxMsgTx.SerializeSize())
	err := ssrtxMsgTx.Serialize(&buf)
	if err != nil {
		t.Errorf("Error serializing the reference sstx: %v", err)
	}
	bufBytes := buf.Bytes()

	// ---------------------------------------------------------------------------
	// Test too many inputs with ssrtxMsgTxTooManyInputs

	var ssrtxTooManyInputs = dcrutil.NewTx(ssrtxMsgTxTooManyInputs)
	ssrtxTooManyInputs.SetTree(wire.TxTreeStake)
	ssrtxTooManyInputs.SetIndex(0)

	err = stake.CheckSSRtx(ssrtxTooManyInputs.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSRtxWrongNumInputs {
		t.Errorf("CheckSSRtx should have returned %v but instead returned %v",
			stake.ErrSSRtxWrongNumInputs, err)
	}
	if stake.IsSSRtx(ssrtxTooManyInputs.MsgTx()) {
		t.Errorf("IsSSRtx claimed an invalid ssrtx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test too many outputs with ssrtxMsgTxTooManyOutputs

	var ssrtxTooManyOutputs = dcrutil.NewTx(ssrtxMsgTxTooManyOutputs)
	ssrtxTooManyOutputs.SetTree(wire.TxTreeStake)
	ssrtxTooManyOutputs.SetIndex(0)

	err = stake.CheckSSRtx(ssrtxTooManyOutputs.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSRtxTooManyOutputs {
		t.Errorf("CheckSSRtx should have returned %v but instead returned %v",
			stake.ErrSSRtxTooManyOutputs, err)
	}
	if stake.IsSSRtx(ssrtxTooManyOutputs.MsgTx()) {
		t.Errorf("IsSSRtx claimed an invalid ssrtx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for bad version of output.
	var ssrtxTxBadVerOut = dcrutil.NewTx(ssrtxMsgTxBadVerOut)
	ssrtxTxBadVerOut.SetTree(wire.TxTreeStake)
	ssrtxTxBadVerOut.SetIndex(0)

	err = stake.CheckSSRtx(ssrtxTxBadVerOut.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSRtxBadOuts {
		t.Errorf("CheckSSRtx should have returned %v but instead returned %v",
			stake.ErrSSRtxBadOuts, err)
	}
	if stake.IsSSRtx(ssrtxTxBadVerOut.MsgTx()) {
		t.Errorf("IsSSRtx claimed an invalid ssrtx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an index 0+ output being not OP_SSRTX tagged
	testRevocOutputUntagged := bytes.Replace(bufBytes,
		[]byte{
			0x1a, 0xbc, 0x76, 0xa9, 0x14, 0xc3, 0x98,
		},
		[]byte{
			0x19, 0x76, 0xa9, 0x14, 0xc3, 0x98,
		},
		1)

	// Deserialize the manipulated tx
	var tx wire.MsgTx
	rbuf := bytes.NewReader(testRevocOutputUntagged)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssrtxTestRevocOutputUntagged = dcrutil.NewTx(&tx)
	ssrtxTestRevocOutputUntagged.SetTree(wire.TxTreeStake)
	ssrtxTestRevocOutputUntagged.SetIndex(0)

	err = stake.CheckSSRtx(ssrtxTestRevocOutputUntagged.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSRtxBadOuts {
		t.Errorf("CheckSSRtx should have returned %v but instead returned %v",
			stake.ErrSSRtxBadOuts, err)
	}
	if stake.IsSSRtx(ssrtxTestRevocOutputUntagged.MsgTx()) {
		t.Errorf("IsSSRtx claimed an invalid ssrtx is valid")
	}

	// ---------------------------------------------------------------------------
	// Wrong tree for inputs test

	// Replace TxTreeStake with TxTreeRegular
	testWrongTreeInputs := bytes.Replace(bufBytes,
		[]byte{0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87, 0x00,
			0x00, 0x00, 0x00, 0x01},
		[]byte{0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87, 0x00,
			0x00, 0x00, 0x00, 0x00},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testWrongTreeInputs)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssrtxWrongTreeIns = dcrutil.NewTx(&tx)
	ssrtxWrongTreeIns.SetTree(wire.TxTreeStake)
	ssrtxWrongTreeIns.SetIndex(0)

	err = stake.CheckSSRtx(ssrtxWrongTreeIns.MsgTx())
	if err.(stake.RuleError).GetCode() != stake.ErrSSRtxWrongTxTree {
		t.Errorf("CheckSSRtx should have returned %v but instead returned %v",
			stake.ErrSSGenWrongTxTree, err)
	}
	if stake.IsSSRtx(ssrtxWrongTreeIns.MsgTx()) {
		t.Errorf("IsSSRtx claimed an invalid ssrtx is valid")
	}
}

// --------------------------------------------------------------------------------
// Minor function testing
func TestGetSSGenBlockVotedOn(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	blockHash, height := stake.SSGenBlockVotedOn(ssgen.MsgTx())

	correctBlockHash, _ := chainhash.NewHash(
		[]byte{
			0x94, 0x8c, 0x76, 0x5a, // 32 byte hash
			0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77,
			0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c,
			0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c,
			0x52, 0xde, 0x3d, 0x7c,
		})

	correctheight := uint32(0x2123e300)

	if !reflect.DeepEqual(blockHash, *correctBlockHash) {
		t.Errorf("Error thrown on TestGetSSGenBlockVotedOn: Looking for "+
			"hash %v, got hash %v", *correctBlockHash, blockHash)
	}

	if height != correctheight {
		t.Errorf("Error thrown on TestGetSSGenBlockVotedOn: Looking for "+
			"height %v, got height %v", correctheight, height)
	}
}

func TestGetSStxStakeOutputInfo(t *testing.T) {
	var sstx = dcrutil.NewTx(sstxMsgTx)
	sstx.SetTree(wire.TxTreeStake)
	sstx.SetIndex(0)

	correctTyp := true

	correctPkh := []byte{0x94, 0x8c, 0x76, 0x5a, // 20 byte address
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
	}

	correctAmt := int64(0x2123e300)

	correctChange := int64(0x2223e300)

	correctRule := true

	correctLimit := uint16(4)

	typs, pkhs, amts, changeAmts, rules, limits :=
		stake.TxSStxStakeOutputInfo(sstx.MsgTx())

	if typs[2] != correctTyp {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"type %v, got type %v", correctTyp, typs[1])
	}

	if !reflect.DeepEqual(pkhs[1], correctPkh) {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"pkh %v, got pkh %v", correctPkh, pkhs[1])
	}

	if amts[1] != correctAmt {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"amount %v, got amount %v", correctAmt, amts[1])
	}

	if changeAmts[1] != correctChange {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"amount %v, got amount %v", correctChange, changeAmts[1])
	}

	if rules[1][0] != correctRule {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"rule %v, got rule %v", correctRule, rules[1][0])
	}

	if limits[1][0] != correctLimit {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"limit %v, got limit %v", correctLimit, rules[1][0])
	}
}

func TestGetSSGenStakeOutputInfo(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	correctTyp := false

	correctpkh := []byte{0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
	}

	correctamt := int64(0x2123e300)

	typs, pkhs, amts, err := stake.TxSSGenStakeOutputInfo(ssgen.MsgTx(),
		&chaincfg.SimNetParams)
	if err != nil {
		t.Errorf("Got unexpected error: %v", err.Error())
	}

	if typs[0] != correctTyp {
		t.Errorf("Error thrown on TestGetSSGenStakeOutputInfo: Looking for "+
			"type %v, got type %v", correctamt, amts[0])
	}

	if !reflect.DeepEqual(pkhs[0], correctpkh) {
		t.Errorf("Error thrown on TestGetSSGenStakeOutputInfo: Looking for "+
			"pkh %v, got pkh %v", correctpkh, pkhs[0])
	}

	if amts[0] != correctamt {
		t.Errorf("Error thrown on TestGetSSGenStakeOutputInfo: Looking for "+
			"amount %v, got amount %v", correctamt, amts[0])
	}
}

func TestGetSSGenVoteBits(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	correctvbs := uint16(0x8c94)

	votebits := stake.SSGenVoteBits(ssgen.MsgTx())

	if correctvbs != votebits {
		t.Errorf("Error thrown on TestGetSSGenVoteBits: Looking for "+
			"vbs % x, got vbs % x", correctvbs, votebits)
	}
}

func TestGetSSGenVersion(t *testing.T) {
	var ssgen = ssgenMsgTx.Copy()

	missingVersion := uint32(stake.VoteConsensusVersionAbsent)
	version := stake.SSGenVersion(ssgen)
	if version != missingVersion {
		t.Errorf("Error thrown on TestGetSSGenVersion: Looking for "+
			"version % x, got version % x", missingVersion, version)
	}

	vbBytes := []byte{0x01, 0x00, 0x01, 0xef, 0xcd, 0xab}
	expectedVersion := uint32(0xabcdef01)
	pkScript, err := txscript.GenerateProvablyPruneableOut(vbBytes)
	if err != nil {
		t.Errorf("GenerateProvablyPruneableOut error %v", err)
	}
	ssgen.TxOut[1].PkScript = pkScript
	version = stake.SSGenVersion(ssgen)

	if version != expectedVersion {
		t.Errorf("Error thrown on TestGetSSGenVersion: Looking for "+
			"version % x, got version % x", expectedVersion, version)
	}
}

func TestGetSSRtxStakeOutputInfo(t *testing.T) {
	var ssrtx = dcrutil.NewTx(ssrtxMsgTx)
	ssrtx.SetTree(wire.TxTreeStake)
	ssrtx.SetIndex(0)

	correctTyp := false

	correctPkh := []byte{0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x33,
	}

	correctAmt := int64(0x2122e300)

	typs, pkhs, amts, err := stake.TxSSRtxStakeOutputInfo(ssrtx.MsgTx(),
		&chaincfg.SimNetParams)
	if err != nil {
		t.Errorf("Got unexpected error: %v", err.Error())
	}

	if typs[0] != correctTyp {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"type %v, got type %v", correctTyp, typs[0])
	}

	if !reflect.DeepEqual(pkhs[0], correctPkh) {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"pkh %v, got pkh %v", correctPkh, pkhs[0])
	}

	if amts[0] != correctAmt {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"amount %v, got amount %v", correctAmt, amts[0])
	}
}

func TestGetSStxNullOutputAmounts(t *testing.T) {
	commitAmts := []int64{int64(0x2122e300),
		int64(0x12000000),
		int64(0x12300000)}
	changeAmts := []int64{int64(0x0122e300),
		int64(0x02000000),
		int64(0x02300000)}
	amtTicket := int64(0x9122e300)

	_, _, err := stake.SStxNullOutputAmounts(
		[]int64{
			int64(0x12000000),
			int64(0x12300000),
		},
		changeAmts,
		amtTicket)

	// len commit to amts != len change amts
	lenErrStr := "amounts was not equal in length " +
		"to change amounts!"
	if err == nil || err.Error() != lenErrStr {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	// too small amount to commit
	_, _, err = stake.SStxNullOutputAmounts(
		commitAmts,
		changeAmts,
		int64(0x00000000))
	tooSmallErrStr := "committed amount was too small!"
	if err == nil || err.Error() != tooSmallErrStr {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	// overspending error
	tooMuchChangeAmts := []int64{int64(0x0122e300),
		int64(0x02000000),
		int64(0x12300001)}

	_, _, err = stake.SStxNullOutputAmounts(
		commitAmts,
		tooMuchChangeAmts,
		int64(0x00000020))
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrSStxBadChangeAmts {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	fees, amts, err := stake.SStxNullOutputAmounts(commitAmts,
		changeAmts,
		amtTicket)

	if err != nil {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	expectedFees := int64(-1361240832)

	if expectedFees != fees {
		t.Errorf("TestGetSStxNullOutputAmounts error, wanted %v, "+
			"but got %v", expectedFees, fees)
	}

	expectedAmts := []int64{int64(0x20000000),
		int64(0x10000000),
		int64(0x10000000),
	}

	if !reflect.DeepEqual(expectedAmts, amts) {
		t.Errorf("TestGetSStxNullOutputAmounts error, wanted %v, "+
			"but got %v", expectedAmts, amts)
	}
}

func TestGetStakeRewards(t *testing.T) {
	// SSGen example with >0 subsidy
	amounts := []int64{int64(21000000),
		int64(11000000),
		int64(10000000),
	}
	amountTicket := int64(42000000)
	subsidy := int64(400000)

	outAmts := stake.CalculateRewards(amounts, amountTicket, subsidy)

	// SSRtx example with 0 subsidy
	expectedAmts := []int64{int64(21200000),
		int64(11104761),
		int64(10095238),
	}

	if !reflect.DeepEqual(expectedAmts, outAmts) {
		t.Errorf("TestGetStakeRewards error, wanted %v, "+
			"but got %v", expectedAmts, outAmts)
	}
}

func TestVerifySStxAmounts(t *testing.T) {
	amounts := []int64{int64(21000000),
		int64(11000000),
		int64(10000000),
	}
	calcAmounts := []int64{int64(21000000),
		int64(11000000),
		int64(10000000),
	}

	// len error for slices
	calcAmountsBad := []int64{int64(11000000),
		int64(10000000),
	}
	err := stake.VerifySStxAmounts(amounts,
		calcAmountsBad)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerSStxAmts {
		t.Errorf("TestVerifySStxAmounts unexpected error: %v", err)
	}

	// non-congruent slices error
	calcAmountsBad = []int64{int64(21000000),
		int64(11000000),
		int64(10000001),
	}
	err = stake.VerifySStxAmounts(amounts,
		calcAmountsBad)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerSStxAmts {
		t.Errorf("TestVerifySStxAmounts unexpected error: %v", err)
	}

	err = stake.VerifySStxAmounts(amounts,
		calcAmounts)
	if err != nil {
		t.Errorf("TestVerifySStxAmounts unexpected error: %v", err)
	}
}

func TestVerifyStakingPkhsAndAmounts(t *testing.T) {
	types := []bool{false, false}
	amounts := []int64{int64(21000000),
		int64(11000000),
	}
	pkhs := [][]byte{
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00},
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x04, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x03}}
	spendTypes := []bool{false, false}
	spendAmounts := []int64{int64(21000000),
		int64(11000000),
	}
	spendPkhs := [][]byte{
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00},
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x04, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x03}}
	spendRules := [][]bool{
		{false, false},
		{false, false}}
	spendLimits := [][]uint16{
		{16, 20},
		{16, 20}}

	// bad types len
	spendTypesBad := []bool{false}
	err := stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypesBad,
		spendPkhs,
		spendAmounts,
		true, // Vote
		spendRules,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyInput {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// bad types
	spendTypesBad = []bool{false, true}
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypesBad,
		spendPkhs,
		spendAmounts,
		true, // Vote
		spendRules,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyOutType {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// len error for amt slices
	spendAmountsBad := []int64{int64(11000111)}
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypes,
		spendPkhs,
		spendAmountsBad,
		true, // Vote
		spendRules,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyInput {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// len error for pks slices
	spendPkhsBad := [][]byte{
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00},
	}
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypes,
		spendPkhsBad,
		spendAmounts,
		true, // Vote
		spendRules,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyInput {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// amount non-equivalence in position 1
	spendAmountsNonequiv := []int64{int64(21000000),
		int64(11000000)}
	spendAmountsNonequiv[1]--

	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypes,
		spendPkhs,
		spendAmountsNonequiv,
		true, // Vote
		spendRules,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyOutputAmt {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// pkh non-equivalence in position 1
	spendPkhsNonequiv := [][]byte{
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00},
		{0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x04, 0x00,
			0x00, 0x01, 0x02, 0x00,
			0x00, 0x01, 0x02, 0x04}}

	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypes,
		spendPkhsNonequiv,
		spendAmounts,
		true, // Vote
		spendRules,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyOutPkhs {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// rule non-equivalence in position 1
	spendRulesNonequivV := [][]bool{
		{false, false},
		{true, false}}
	spendAmountsNonequivV := []int64{int64(21000000),
		int64(10934463)}
	spendAmountsNonequivVTooBig := []int64{int64(21000000),
		int64(11000001)}

	spendRulesNonequivR := [][]bool{
		{false, false},
		{false, true}}
	spendAmountsNonequivR := []int64{int64(21000000),
		int64(9951423)}

	// vote
	// original amount: 11000000
	// with the flag enabled, the minimum allowed to be spent is
	// 11000000 - 1 << 16 = 10934464
	// So, 10934464 should pass while 10934463 should fail.
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		spendAmountsNonequivV,
		spendTypes,
		spendPkhs,
		amounts,
		true, // Vote
		spendRulesNonequivV,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyTooMuchFees {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// original amount: 11000000
	// the maximum allows to be spent is 11000000
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		spendAmountsNonequivVTooBig,
		spendTypes,
		spendPkhs,
		amounts,
		true, // Vote
		spendRulesNonequivV,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifySpendTooMuch {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// revocation
	// original amount: 11000000
	// with the flag enabled, the minimum allowed to be spent is
	// 11000000 - 1 << 20 = 9951424
	// So, 9951424 should pass while 9951423 should fail.
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		spendAmountsNonequivR,
		spendTypes,
		spendPkhs,
		amounts,
		false, // Revocation
		spendRulesNonequivR,
		spendLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyTooMuchFees {
		t.Errorf("TestVerifyStakingPkhsAndAmounts unexpected error: %v", err)
	}

	// correct verification
	err = stake.VerifyStakingPkhsAndAmounts(types,
		pkhs,
		amounts,
		spendTypes,
		spendPkhs,
		spendAmounts,
		true, // Vote
		spendRules,
		spendLimits)
	if err != nil {
		t.Errorf("TestVerifySStxAmounts unexpected error: %v", err)
	}
}

func TestVerifyRealTxs(t *testing.T) {
	// Load an SStx and the SSRtx that spends to test some real fee situation
	// and confirm the functionality of the functions used.
	hexSstx, _ := hex.DecodeString("010000000267cfaa9ce3a50977dcd1015f4f" +
		"ce330071a3a9b855210e6646f6434caebda5a60200000001fffffffff6e6004" +
		"fd4a0a8d5823c99be0a66a5f9a89c3dd4f7cbf76880098b8ca9d80b0e020000" +
		"0001ffffffff05a42df60c0000000000001aba76a914c96206f8a3976057b2e" +
		"b846d46d4a909972fc7c788ac00000000000000000000206a1ec96206f8a397" +
		"6057b2eb846d46d4a909972fc7c780fe210a000000000054000000000000000" +
		"000001abd76a914c96206f8a3976057b2eb846d46d4a909972fc7c788ac0000" +
		"0000000000000000206a1ec96206f8a3976057b2eb846d46d4a909972fc7c70" +
		"c33d40200000000005474cb4d070000000000001abd76a914c96206f8a39760" +
		"57b2eb846d46d4a909972fc7c788ac00000000000000000280fe210a0000000" +
		"013030000000000006a47304402200dbc873e69571a4516c4ef869d856386f9" +
		"86c8543c0bc9f372ecd22c8606ccb102200f87a8f1b316b7675dfd1706eb22f" +
		"331cea14d2e2d5f2c1d88173881a0cd4a04012102716f806d1156d20b9b2482" +
		"2bff88549b510f400473536d3ea8d188b9fbe3835680fe210a0000000002030" +
		"000040000006b483045022100bc7d0b7aa2c6610b7639f492fa556954ebc52a" +
		"9dca5a417be4705ab424255ccd02200a0ccba2e2b7391b93b927f35150c1253" +
		"2bdc6f27a8e9eb0bd0bfbc8b9ab13a5012102716f806d1156d20b9b24822bff" +
		"88549b510f400473536d3ea8d188b9fbe38356")
	sstxMtx := new(wire.MsgTx)
	sstxMtx.FromBytes(hexSstx)
	sstxTx := dcrutil.NewTx(sstxMtx)
	sstxTypes, sstxAddrs, sstxAmts, _, sstxRules, sstxLimits :=
		stake.TxSStxStakeOutputInfo(sstxTx.MsgTx())

	hexSsrtx, _ := hex.DecodeString("010000000147f4453f244f2589551aea7c714d" +
		"771053b667c6612616e9c8fc0e68960a9a100000000001ffffffff0270d7210a00" +
		"00000000001abc76a914c96206f8a3976057b2eb846d46d4a909972fc7c788ac0c" +
		"33d4020000000000001abc76a914c96206f8a3976057b2eb846d46d4a909972fc7" +
		"c788ac000000000000000001ffffffffffffffff00000000ffffffff6b48304502" +
		"2100d01c52c3f0c27166e3633d93b5ba821365a73f761e23bb04cc8061a28ab1bd" +
		"7d02202bd65a6d16aaefe8b7f56378d58da6650f2e4b20bd5cb659dc9e842ce2d9" +
		"15e6012102716f806d1156d20b9b24822bff88549b510f400473536d3ea8d188b9" +
		"fbe38356")
	ssrtxMtx := new(wire.MsgTx)
	ssrtxMtx.FromBytes(hexSsrtx)
	ssrtxTx := dcrutil.NewTx(ssrtxMtx)

	ssrtxTypes, ssrtxAddrs, ssrtxAmts, err :=
		stake.TxSSRtxStakeOutputInfo(ssrtxTx.MsgTx(), &chaincfg.TestNet2Params)
	if err != nil {
		t.Errorf("Unexpected GetSSRtxStakeOutputInfo error: %v", err.Error())
	}

	ssrtxCalcAmts := stake.CalculateRewards(sstxAmts, sstxMtx.TxOut[0].Value,
		int64(0))

	// Here an error is thrown because the second output spends too much.
	// Ticket price: 217460132
	// 1: 170000000 - 170000000. 169999218 allowed back (-782 atoms)
	// 2: 170000000 -  47461132. 122538868 Change. Paid 1000 fees total.
	//    47460913 allowed back (-219 atoms for fee).
	// In this test the second output spends 47461132, which is more than
	// allowed.
	err = stake.VerifyStakingPkhsAndAmounts(sstxTypes,
		sstxAddrs,
		ssrtxAmts,
		ssrtxTypes,
		ssrtxAddrs,
		ssrtxCalcAmts,
		false, // Revocation
		sstxRules,
		sstxLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifySpendTooMuch {
		t.Errorf("No or unexpected VerifyStakingPkhsAndAmounts error: %v",
			err.Error())
	}

	// Correct this and make sure it passes.
	ssrtxTx.MsgTx().TxOut[1].Value = 47460913
	sstxTypes, sstxAddrs, sstxAmts, _, sstxRules, sstxLimits =
		stake.TxSStxStakeOutputInfo(sstxTx.MsgTx())
	ssrtxTypes, ssrtxAddrs, ssrtxAmts, err =
		stake.TxSSRtxStakeOutputInfo(ssrtxTx.MsgTx(), &chaincfg.TestNet2Params)
	if err != nil {
		t.Errorf("Unexpected GetSSRtxStakeOutputInfo error: %v", err.Error())
	}
	ssrtxCalcAmts = stake.CalculateRewards(sstxAmts, sstxMtx.TxOut[0].Value,
		int64(0))
	err = stake.VerifyStakingPkhsAndAmounts(sstxTypes,
		sstxAddrs,
		ssrtxAmts,
		ssrtxTypes,
		ssrtxAddrs,
		ssrtxCalcAmts,
		false, // Revocation
		sstxRules,
		sstxLimits)
	if err != nil {
		t.Errorf("Unexpected VerifyStakingPkhsAndAmounts error: %v",
			err)
	}

	// Spend too much fees for the limit in the first output and
	// make sure it fails.
	ssrtxTx.MsgTx().TxOut[0].Value = 0
	sstxTypes, sstxAddrs, sstxAmts, _, sstxRules, sstxLimits =
		stake.TxSStxStakeOutputInfo(sstxTx.MsgTx())
	ssrtxTypes, ssrtxAddrs, ssrtxAmts, err =
		stake.TxSSRtxStakeOutputInfo(ssrtxTx.MsgTx(), &chaincfg.TestNet2Params)
	if err != nil {
		t.Errorf("Unexpected GetSSRtxStakeOutputInfo error: %v", err.Error())
	}
	ssrtxCalcAmts = stake.CalculateRewards(sstxAmts, sstxMtx.TxOut[0].Value,
		int64(0))
	err = stake.VerifyStakingPkhsAndAmounts(sstxTypes,
		sstxAddrs,
		ssrtxAmts,
		ssrtxTypes,
		ssrtxAddrs,
		ssrtxCalcAmts,
		false, // Revocation
		sstxRules,
		sstxLimits)
	if err == nil || err.(stake.RuleError).GetCode() !=
		stake.ErrVerifyTooMuchFees {
		t.Errorf("No or unexpected VerifyStakingPkhsAndAmounts error: %v",
			err.Error())
	}

	// Use everything as fees and make sure both participants are paid
	// equally for their contibutions. Both inputs to the SStx are the
	// same size, so this is possible.
	copy(sstxTx.MsgTx().TxOut[3].PkScript, sstxTx.MsgTx().TxOut[1].PkScript)
	sstxTx.MsgTx().TxOut[4].Value = 0
	ssrtxTx.MsgTx().TxOut[0].Value = 108730066
	ssrtxTx.MsgTx().TxOut[1].Value = 108730066
	sstxTypes, sstxAddrs, sstxAmts, _, sstxRules, sstxLimits =
		stake.TxSStxStakeOutputInfo(sstxTx.MsgTx())
	ssrtxTypes, ssrtxAddrs, ssrtxAmts, err =
		stake.TxSSRtxStakeOutputInfo(ssrtxTx.MsgTx(), &chaincfg.TestNet2Params)
	if err != nil {
		t.Errorf("Unexpected GetSSRtxStakeOutputInfo error: %v", err.Error())
	}
	ssrtxCalcAmts = stake.CalculateRewards(sstxAmts, sstxMtx.TxOut[0].Value,
		int64(0))
	err = stake.VerifyStakingPkhsAndAmounts(sstxTypes,
		sstxAddrs,
		ssrtxAmts,
		ssrtxTypes,
		ssrtxAddrs,
		ssrtxCalcAmts,
		false, // Revocation
		sstxRules,
		sstxLimits)
	if err != nil {
		t.Errorf("Unexpected VerifyStakingPkhsAndAmounts error: %v",
			err.Error())
	}
	if ssrtxCalcAmts[0] != ssrtxCalcAmts[1] {
		t.Errorf("Unexpected ssrtxCalcAmts; both values should be same but "+
			"got %v and %v", ssrtxCalcAmts[0], ssrtxCalcAmts[1])
	}
}

// --------------------------------------------------------------------------------
// TESTING VARIABLES BEGIN HERE

// sstxTxIn is the first input in the reference valid sstx
var sstxTxIn = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash: chainhash.Hash([32]byte{ // Make go vet happy.
			0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
			0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
			0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
			0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
		}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
		Index: 0,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0x49, // OP_DATA_73
		0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
		0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
		0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
		0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
		0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
		0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
		0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
		0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
		0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
		0x01, // 73-byte signature
		0x41, // OP_DATA_65
		0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
		0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
		0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
		0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
		0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
		0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
		0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
		0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
		0xd3, // 65-byte pubkey
	},
	Sequence: 0xffffffff,
}

// sstxTxOut0 is the first output in the reference valid sstx
var sstxTxOut0 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xba, // OP_SSTX
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// sstxTxOut1 is the second output in the reference valid sstx
var sstxTxOut1 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,                   // OP_RETURN
		0x1e,                   // 30 bytes to be pushed
		0x94, 0x8c, 0x76, 0x5a, // 20 byte address
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0x00, 0xe3, 0x23, 0x21, // Transaction amount
		0x00, 0x00, 0x00, 0x00,
		0x44, 0x3f, // Fee limits
	},
}

// sstxTxOut2 is the third output in the reference valid sstx
var sstxTxOut2 = wire.TxOut{
	Value:   0x2223e300,
	Version: 0x0000,
	PkScript: []byte{
		0xbd, // OP_SSTXCHANGE
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// sstxTxOut3 is another output in an SStx, this time instruction to pay to
// a P2SH output
var sstxTxOut3 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,                   // OP_RETURN
		0x1e,                   // 30 bytes to be pushed
		0x94, 0x8c, 0x76, 0x5a, // 20 byte address
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0x00, 0xe3, 0x23, 0x21, // Transaction amount
		0x00, 0x00, 0x00, 0x80, // Last byte flagged
		0x44, 0x3f, // Fee limits
	},
}

// sstxTxOut4 is the another output in the reference valid sstx, and pays change
// to a P2SH address
var sstxTxOut4 = wire.TxOut{
	Value:   0x2223e300,
	Version: 0x0000,
	PkScript: []byte{
		0xbd, // OP_SSTXCHANGE
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// sstxTxOut4VerBad is the third output in the reference valid sstx, with a
// bad version.
var sstxTxOut4VerBad = wire.TxOut{
	Value:   0x2223e300,
	Version: 0x1234,
	PkScript: []byte{
		0xbd, // OP_SSTXCHANGE
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// sstxMsgTx is a valid SStx MsgTx with an input and outputs and is used in various
// tests
var sstxMsgTx = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
		&sstxTxIn,
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0,
		&sstxTxOut1,
		&sstxTxOut2, // emulate change address
		&sstxTxOut1,
		&sstxTxOut2, // emulate change address
		&sstxTxOut3, // P2SH
		&sstxTxOut4, // P2SH change
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxMsgTxExtraInputs is an invalid SStx MsgTx with too many inputs
var sstxMsgTxExtraInput = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0,
		&sstxTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxMsgTxExtraOutputs is an invalid SStx MsgTx with too many outputs
var sstxMsgTxExtraOutputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxMismatchedInsOuts is an invalid SStx MsgTx with too many outputs for the
// number of inputs it has
var sstxMismatchedInsOuts = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut2, &sstxTxOut1, &sstxTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxBadVersionOut is an invalid SStx MsgTx with an output containing a bad
// version.
var sstxBadVersionOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
		&sstxTxIn,
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0,
		&sstxTxOut1,
		&sstxTxOut2,       // emulate change address
		&sstxTxOut1,       // 3
		&sstxTxOut2,       // 4
		&sstxTxOut3,       // 5 P2SH
		&sstxTxOut4VerBad, // 6 P2SH change
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxNullDataMissing is an invalid SStx MsgTx with no address push in the second
// output
var sstxNullDataMissing = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut0, &sstxTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxNullDataMisplaced is an invalid SStx MsgTx that has the commitment and
// change outputs swapped
var sstxNullDataMisplaced = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut2, &sstxTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenTxIn0 is the 0th position input in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxIn0 = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04,
	},
	BlockHeight: wire.NullBlockHeight,
	BlockIndex:  wire.NullBlockIndex,
	Sequence:    0xffffffff,
}

// ssgenTxIn1 is the 1st position input in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxIn1 = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash: chainhash.Hash([32]byte{ // Make go vet happy.
			0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
			0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
			0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
			0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
		}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
		Index: 0,
		Tree:  wire.TxTreeStake,
	},
	SignatureScript: []byte{
		0x49, // OP_DATA_73
		0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
		0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
		0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
		0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
		0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
		0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
		0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
		0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
		0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
		0x01, // 73-byte signature
		0x41, // OP_DATA_65
		0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
		0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
		0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
		0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
		0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
		0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
		0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
		0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
		0xd3, // 65-byte pubkey
	},
	Sequence: 0xffffffff,
}

// ssgenTxOut0 is the 0th position output in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxOut0 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,                   // OP_RETURN
		0x24,                   // 36 bytes to be pushed
		0x94, 0x8c, 0x76, 0x5a, // 32 byte hash
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0x52, 0xde, 0x3d, 0x7c,
		0x00, 0xe3, 0x23, 0x21, // 4 byte height
	},
}

// ssgenTxOut1 is the 1st position output in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxOut1 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,       // OP_RETURN
		0x02,       // 2 bytes to be pushed
		0x94, 0x8c, // Vote bits
	},
}

// ssgenTxOut2 is the 2nd position output in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxOut2 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xbb, // OP_SSGEN
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// ssgenTxOut3 is a P2SH output
var ssgenTxOut3 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xbb, // OP_SSGEN
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssgenTxOut3BadVer is a P2SH output with a bad version.
var ssgenTxOut3BadVer = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0100,
	PkScript: []byte{
		0xbb, // OP_SSGEN
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssgenMsgTx is a valid SSGen MsgTx with an input and outputs and is used in
// various testing scenarios
var ssgenMsgTx = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxExtraInput is an invalid SSGen MsgTx with too many inputs
var ssgenMsgTxExtraInput = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxExtraOutputs is an invalid SSGen MsgTx with too many outputs
var ssgenMsgTxExtraOutputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxStakeBaseWrong is an invalid SSGen tx with the stakebase in the wrong
// position
var ssgenMsgTxStakeBaseWrong = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn1,
		&ssgenTxIn0,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxBadVerOut is an invalid SSGen tx that contains an output with a bad
// version
var ssgenMsgTxBadVerOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3BadVer,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxWrongZeroethOut is an invalid SSGen tx with the first output being not
// an OP_RETURN push
var ssgenMsgTxWrongZeroethOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut2,
		&ssgenTxOut1,
		&ssgenTxOut0,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxWrongFirstOut is an invalid SSGen tx with the second output being not
// an OP_RETURN push
var ssgenMsgTxWrongFirstOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut2,
		&ssgenTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssrtxTxIn is the 0th position input in a valid SSRtx tx used to test out the
// IsSSRtx function
var ssrtxTxIn = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash: chainhash.Hash([32]byte{ // Make go vet happy.
			0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
			0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
			0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
			0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
		}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
		Index: 0,
		Tree:  wire.TxTreeStake,
	},
	SignatureScript: []byte{
		0x49, // OP_DATA_73
		0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
		0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
		0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
		0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
		0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
		0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
		0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
		0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
		0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
		0x01, // 73-byte signature
		0x41, // OP_DATA_65
		0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
		0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
		0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
		0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
		0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
		0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
		0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
		0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
		0xd3, // 65-byte pubkey
	},
	Sequence: 0xffffffff,
}

// ssrtxTxOut is the 0th position output in a valid SSRtx tx used to test out the
// IsSSRtx function
var ssrtxTxOut = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0xbc, // OP_SSGEN
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x33,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// ssrtxTxOut2 is a P2SH output
var ssrtxTxOut2 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xbc, // OP_SSRTX
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssrtxTxOut2BadVer is a P2SH output with a non-default script version
var ssrtxTxOut2BadVer = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0100,
	PkScript: []byte{
		0xbc, // OP_SSRTX
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssrtxMsgTx is a valid SSRtx MsgTx with an input and outputs and is used in
// various testing scenarios
var ssrtxMsgTx = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut,
		&ssrtxTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssrtxMsgTx is a valid SSRtx MsgTx with an input and outputs and is used in
// various testing scenarios
var ssrtxMsgTxTooManyInputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssrtxMsgTx is a valid SSRtx MsgTx with an input and outputs and is used in
// various testing scenarios
var ssrtxMsgTxTooManyOutputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
	},
	LockTime: 0,
	Expiry:   0,
}

var ssrtxMsgTxBadVerOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut,
		&ssrtxTxOut2BadVer,
	},
	LockTime: 0,
	Expiry:   0,
}

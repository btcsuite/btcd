package blockchain

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// DoStxoTest does a test on a simulated blockchain to ensure that the data
// stored in the STXO buckets is not corrupt.
func (b *BlockChain) DoStxoTest() error {
	err := b.db.View(func(dbTx database.Tx) error {
		for i := int64(2); i <= b.bestNode.height; i++ {
			block, err := dbFetchBlockByHeight(dbTx, i)
			if err != nil {
				return err
			}

			parent, err := dbFetchBlockByHeight(dbTx, i-1)
			if err != nil {
				return err
			}

			ntx := countSpentOutputs(block, parent)
			stxos, err := dbFetchSpendJournalEntry(dbTx, block, parent)
			if err != nil {
				return err
			}

			if int(ntx) != len(stxos) {
				return fmt.Errorf("bad number of stxos calculated at "+
					"height %v, got %v expected %v",
					i, len(stxos), int(ntx))
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// DebugBlockHeaderString dumps a verbose message containing information about
// the block header of a block.
func DebugBlockHeaderString(chainParams *chaincfg.Params,
	block *dcrutil.Block) string {
	bh := block.MsgBlock().Header

	var buffer bytes.Buffer

	str := fmt.Sprintf("Version: %v\n", bh.Version)
	buffer.WriteString(str)

	str = fmt.Sprintf("Previous block: %v\n", bh.PrevBlock)
	buffer.WriteString(str)

	str = fmt.Sprintf("Merkle root (reg): %v\n", bh.MerkleRoot)
	buffer.WriteString(str)

	str = fmt.Sprintf("Merkle root (stk): %v\n", bh.StakeRoot)
	buffer.WriteString(str)

	str = fmt.Sprintf("VoteBits: %v\n", bh.VoteBits)
	buffer.WriteString(str)

	str = fmt.Sprintf("FinalState: %v\n", bh.FinalState)
	buffer.WriteString(str)

	str = fmt.Sprintf("Voters: %v\n", bh.Voters)
	buffer.WriteString(str)

	str = fmt.Sprintf("FreshStake: %v\n", bh.FreshStake)
	buffer.WriteString(str)

	str = fmt.Sprintf("Revocations: %v\n", bh.Revocations)
	buffer.WriteString(str)

	str = fmt.Sprintf("PoolSize: %v\n", bh.PoolSize)
	buffer.WriteString(str)

	str = fmt.Sprintf("Timestamp: %v\n", bh.Timestamp)
	buffer.WriteString(str)

	bitsBig := CompactToBig(bh.Bits)
	if bitsBig.Cmp(bigZero) != 0 {
		bitsBig.Div(chainParams.PowLimit, bitsBig)
	}
	diff := bitsBig.Int64()
	str = fmt.Sprintf("Bits: %v (Difficulty: %v)\n", bh.Bits, diff)
	buffer.WriteString(str)

	str = fmt.Sprintf("SBits: %v (In coins: %v)\n", bh.SBits,
		float64(bh.SBits)/dcrutil.AtomsPerCoin)
	buffer.WriteString(str)

	str = fmt.Sprintf("Nonce: %v \n", bh.Nonce)
	buffer.WriteString(str)

	str = fmt.Sprintf("Height: %v \n", bh.Height)
	buffer.WriteString(str)

	str = fmt.Sprintf("Size: %v \n", bh.Size)
	buffer.WriteString(str)

	return buffer.String()
}

// DebugBlockString dumps a verbose message containing information about
// the transactions of a block.
func DebugBlockString(block *dcrutil.Block) string {
	if block == nil {
		return "block pointer nil"
	}

	var buffer bytes.Buffer

	hash := block.Sha()

	str := fmt.Sprintf("Block Header: %v Height: %v \n",
		hash, block.Height())
	buffer.WriteString(str)

	str = fmt.Sprintf("Block contains %v regular transactions "+
		"and %v stake transactions \n",
		len(block.Transactions()),
		len(block.STransactions()))
	buffer.WriteString(str)

	str = fmt.Sprintf("List of regular transactions \n")
	buffer.WriteString(str)

	for i, tx := range block.Transactions() {
		str = fmt.Sprintf("Index: %v, Hash: %v \n", i, tx.Sha())
		buffer.WriteString(str)
	}

	if len(block.STransactions()) == 0 {
		return buffer.String()
	}

	str = fmt.Sprintf("List of stake transactions \n")
	buffer.WriteString(str)

	for i, stx := range block.STransactions() {
		txTypeStr := ""
		txType := stake.DetermineTxType(stx)
		switch txType {
		case stake.TxTypeSStx:
			txTypeStr = "SStx"
		case stake.TxTypeSSGen:
			txTypeStr = "SSGen"
		case stake.TxTypeSSRtx:
			txTypeStr = "SSRtx"
		default:
			txTypeStr = "Error"
		}

		str = fmt.Sprintf("Index: %v, Type: %v, Hash: %v \n",
			i, txTypeStr, stx.Sha())
		buffer.WriteString(str)
	}

	return buffer.String()
}

// DebugMsgTxString dumps a verbose message containing information about the
// contents of a transaction.
func DebugMsgTxString(msgTx *wire.MsgTx) string {
	tx := dcrutil.NewTx(msgTx)
	isSStx, _ := stake.IsSStx(tx)
	isSSGen, _ := stake.IsSSGen(tx)
	var sstxType []bool
	var sstxPkhs [][]byte
	var sstxAmts []int64
	var sstxRules [][]bool
	var sstxLimits [][]uint16

	if isSStx {
		sstxType, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits =
			stake.TxSStxStakeOutputInfo(tx)
	}

	var buffer bytes.Buffer

	hash := msgTx.TxSha()
	str := fmt.Sprintf("Transaction hash: %v, Version %v, Locktime: %v, "+
		"Expiry %v\n\n", hash, msgTx.Version, msgTx.LockTime, msgTx.Expiry)
	buffer.WriteString(str)

	str = fmt.Sprintf("==INPUTS==\nNumber of inputs: %v\n\n",
		len(msgTx.TxIn))
	buffer.WriteString(str)

	for i, input := range msgTx.TxIn {
		str = fmt.Sprintf("Input number: %v\n", i)
		buffer.WriteString(str)

		str = fmt.Sprintf("Previous outpoint hash: %v, ",
			input.PreviousOutPoint.Hash)
		buffer.WriteString(str)

		str = fmt.Sprintf("Previous outpoint index: %v, ",
			input.PreviousOutPoint.Index)
		buffer.WriteString(str)

		str = fmt.Sprintf("Previous outpoint tree: %v \n",
			input.PreviousOutPoint.Tree)
		buffer.WriteString(str)

		str = fmt.Sprintf("Sequence: %v \n",
			input.Sequence)
		buffer.WriteString(str)

		str = fmt.Sprintf("ValueIn: %v \n",
			input.ValueIn)
		buffer.WriteString(str)

		str = fmt.Sprintf("BlockHeight: %v \n",
			input.BlockHeight)
		buffer.WriteString(str)

		str = fmt.Sprintf("BlockIndex: %v \n",
			input.BlockIndex)
		buffer.WriteString(str)

		str = fmt.Sprintf("Raw signature script: %x \n", input.SignatureScript)
		buffer.WriteString(str)

		sigScr, _ := txscript.DisasmString(input.SignatureScript)
		str = fmt.Sprintf("Disasmed signature script: %v \n\n",
			sigScr)
		buffer.WriteString(str)
	}

	str = fmt.Sprintf("==OUTPUTS==\nNumber of outputs: %v\n\n",
		len(msgTx.TxOut))
	buffer.WriteString(str)

	for i, output := range msgTx.TxOut {
		str = fmt.Sprintf("Output number: %v\n", i)
		buffer.WriteString(str)

		coins := float64(output.Value) / 1e8
		str = fmt.Sprintf("Output amount: %v atoms or %v coins\n", output.Value,
			coins)
		buffer.WriteString(str)

		// SStx OP_RETURNs, dump pkhs and amts committed
		if isSStx && i != 0 && i%2 == 1 {
			coins := float64(sstxAmts[i/2]) / 1e8
			str = fmt.Sprintf("SStx commit amount: %v atoms or %v coins\n",
				sstxAmts[i/2], coins)
			buffer.WriteString(str)
			str = fmt.Sprintf("SStx commit address: %x\n",
				sstxPkhs[i/2])
			buffer.WriteString(str)
			str = fmt.Sprintf("SStx address type is P2SH: %v\n",
				sstxType[i/2])
			buffer.WriteString(str)

			str = fmt.Sprintf("SStx all address types is P2SH: %v\n",
				sstxType)
			buffer.WriteString(str)

			str = fmt.Sprintf("Voting is fee limited: %v\n",
				sstxLimits[i/2][0])
			buffer.WriteString(str)
			if sstxRules[i/2][0] {
				str = fmt.Sprintf("Voting limit imposed: %v\n",
					sstxLimits[i/2][0])
				buffer.WriteString(str)
			}

			str = fmt.Sprintf("Revoking is fee limited: %v\n",
				sstxRules[i/2][1])
			buffer.WriteString(str)

			if sstxRules[i/2][1] {
				str = fmt.Sprintf("Voting limit imposed: %v\n",
					sstxLimits[i/2][1])
				buffer.WriteString(str)
			}
		}

		// SSGen block/block height OP_RETURN.
		if isSSGen && i == 0 {
			blkHash, blkHeight, _ := stake.SSGenBlockVotedOn(tx)
			str = fmt.Sprintf("SSGen block hash voted on: %v, height: %v\n",
				blkHash, blkHeight)
			buffer.WriteString(str)
		}

		if isSSGen && i == 1 {
			vb := stake.SSGenVoteBits(tx)
			str = fmt.Sprintf("SSGen vote bits: %v\n", vb)
			buffer.WriteString(str)
		}

		str = fmt.Sprintf("Raw script: %x \n", output.PkScript)
		buffer.WriteString(str)

		scr, _ := txscript.DisasmString(output.PkScript)
		str = fmt.Sprintf("Disasmed script: %v \n\n", scr)
		buffer.WriteString(str)
	}

	return buffer.String()
}

// DebugUtxoEntryData returns a string containing information about the data
// stored in the given UtxoEntry.
func DebugUtxoEntryData(hash chainhash.Hash, utx *UtxoEntry) string {
	var buffer bytes.Buffer
	str := fmt.Sprintf("Hash: %v\n", hash)
	buffer.WriteString(str)
	if utx == nil {
		str := fmt.Sprintf("MISSING\n\n")
		buffer.WriteString(str)
		return buffer.String()
	}

	str = fmt.Sprintf("Height: %v\n", utx.height)
	buffer.WriteString(str)
	str = fmt.Sprintf("Index: %v\n", utx.index)
	buffer.WriteString(str)
	str = fmt.Sprintf("TxVersion: %v\n", utx.txVersion)
	buffer.WriteString(str)
	str = fmt.Sprintf("TxType: %v\n", utx.txType)
	buffer.WriteString(str)
	str = fmt.Sprintf("IsCoinbase: %v\n", utx.isCoinBase)
	buffer.WriteString(str)
	str = fmt.Sprintf("HasExpiry: %v\n", utx.hasExpiry)
	buffer.WriteString(str)
	str = fmt.Sprintf("FullySpent: %v\n", utx.IsFullySpent())
	buffer.WriteString(str)
	str = fmt.Sprintf("StakeExtra: %x\n\n", utx.stakeExtra)
	buffer.WriteString(str)

	outputOrdered := make([]int, 0, len(utx.sparseOutputs))
	for outputIndex := range utx.sparseOutputs {
		outputOrdered = append(outputOrdered, int(outputIndex))
	}
	sort.Ints(outputOrdered)
	for _, idx := range outputOrdered {
		utxo := utx.sparseOutputs[uint32(idx)]
		str = fmt.Sprintf("Output index: %v\n", idx)
		buffer.WriteString(str)
		str = fmt.Sprintf("Amount: %v\n", utxo.amount)
		buffer.WriteString(str)
		str = fmt.Sprintf("ScriptVersion: %v\n", utxo.scriptVersion)
		buffer.WriteString(str)
		str = fmt.Sprintf("Script: %x\n", utxo.pkScript)
		buffer.WriteString(str)
		str = fmt.Sprintf("Spent: %v\n", utxo.spent)
		buffer.WriteString(str)
	}
	str = fmt.Sprintf("\n")
	buffer.WriteString(str)

	return buffer.String()
}

// DebugUtxoViewpointData returns a string containing information about the data
// stored in the given UtxoView.
func DebugUtxoViewpointData(uv *UtxoViewpoint) string {
	if uv == nil {
		return ""
	}

	var buffer bytes.Buffer

	for hash, utx := range uv.entries {
		buffer.WriteString(DebugUtxoEntryData(hash, utx))
	}

	return buffer.String()
}

// DebugStxoData returns a string containing information about the data
// stored in the given STXO.
func DebugStxoData(stx *spentTxOut) string {
	if stx == nil {
		return ""
	}

	var buffer bytes.Buffer

	str := fmt.Sprintf("amount: %v\n", stx.amount)
	buffer.WriteString(str)
	str = fmt.Sprintf("scriptVersion: %v\n", stx.scriptVersion)
	buffer.WriteString(str)
	str = fmt.Sprintf("pkScript: %x\n", stx.pkScript)
	buffer.WriteString(str)
	str = fmt.Sprintf("compressed: %v\n", stx.compressed)
	buffer.WriteString(str)
	str = fmt.Sprintf("stakeExtra: %x\n", stx.stakeExtra)
	buffer.WriteString(str)
	str = fmt.Sprintf("txVersion: %v\n", stx.txVersion)
	buffer.WriteString(str)
	str = fmt.Sprintf("height: %v\n", stx.height)
	buffer.WriteString(str)
	str = fmt.Sprintf("index: %v\n", stx.index)
	buffer.WriteString(str)
	str = fmt.Sprintf("isCoinbase: %v\n", stx.isCoinBase)
	buffer.WriteString(str)
	str = fmt.Sprintf("hasExpiry: %v\n", stx.hasExpiry)
	buffer.WriteString(str)
	str = fmt.Sprintf("txType: %v\n", stx.txType)
	buffer.WriteString(str)
	str = fmt.Sprintf("fullySpent: %v\n", stx.txFullySpent)
	buffer.WriteString(str)

	str = fmt.Sprintf("\n")
	buffer.WriteString(str)

	return buffer.String()
}

// DebugStxosData returns a string containing information about the data
// stored in the given slice of STXOs.
func DebugStxosData(stxs []spentTxOut) string {
	if stxs == nil {
		return ""
	}
	var buffer bytes.Buffer

	// Iterate backwards.
	var str string
	for i := len(stxs) - 1; i >= 0; i-- {
		str = fmt.Sprintf("STX index %v\n", i)
		buffer.WriteString(str)
		str = fmt.Sprintf("amount: %v\n", stxs[i].amount)
		buffer.WriteString(str)
		str = fmt.Sprintf("scriptVersion: %v\n", stxs[i].scriptVersion)
		buffer.WriteString(str)
		str = fmt.Sprintf("pkScript: %x\n", stxs[i].pkScript)
		buffer.WriteString(str)
		str = fmt.Sprintf("compressed: %v\n", stxs[i].compressed)
		buffer.WriteString(str)
		str = fmt.Sprintf("stakeExtra: %x\n", stxs[i].stakeExtra)
		buffer.WriteString(str)
		str = fmt.Sprintf("txVersion: %v\n", stxs[i].txVersion)
		buffer.WriteString(str)
		str = fmt.Sprintf("height: %v\n", stxs[i].height)
		buffer.WriteString(str)
		str = fmt.Sprintf("index: %v\n", stxs[i].index)
		buffer.WriteString(str)
		str = fmt.Sprintf("isCoinbase: %v\n", stxs[i].isCoinBase)
		buffer.WriteString(str)
		str = fmt.Sprintf("hasExpiry: %v\n", stxs[i].hasExpiry)
		buffer.WriteString(str)
		str = fmt.Sprintf("txType: %v\n", stxs[i].txType)
		buffer.WriteString(str)
		str = fmt.Sprintf("fullySpent: %v\n\n", stxs[i].txFullySpent)
		buffer.WriteString(str)
	}
	str = fmt.Sprintf("\n")
	buffer.WriteString(str)

	return buffer.String()
}

package blockchain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

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

// DebugTicketDataString writes the contents of a ticket data struct
// as a string.
func DebugTicketDataString(td *stake.TicketData) string {
	var buffer bytes.Buffer

	str := fmt.Sprintf("SStxHash: %v\n", td.SStxHash)
	buffer.WriteString(str)

	str = fmt.Sprintf("SpendHash: %v\n", td.SpendHash)
	buffer.WriteString(str)

	str = fmt.Sprintf("BlockHeight: %v\n", td.BlockHeight)
	buffer.WriteString(str)

	str = fmt.Sprintf("Prefix: %v\n", td.Prefix)
	buffer.WriteString(str)

	str = fmt.Sprintf("Missed: %v\n", td.Missed)
	buffer.WriteString(str)

	str = fmt.Sprintf("Expired: %v\n", td.Expired)
	buffer.WriteString(str)

	return buffer.String()
}

// DebugTicketDBLiveString prints out the number of tickets in each
// bucket of the ticket database as a string.
func DebugTicketDBLiveString(tmdb *stake.TicketDB,
	chainParams *chaincfg.Params) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\n")

	for i := 0; i < stake.BucketsSize; i++ {
		bucketTickets, err := tmdb.DumpLiveTickets(uint8(i))
		if err != nil {
			return "", err
		}

		str := fmt.Sprintf("%v: %v\t", i, len(bucketTickets))
		buffer.WriteString(str)

		// Add newlines.
		if (i+1)%4 == 0 {
			buffer.WriteString("\n")
		}
	}

	return buffer.String(), nil
}

// DebugTicketDBLiveBucketString returns a string containing the ticket hashes
// found in a specific bucket of the live ticket database. If the verbose flag
// is called, it dumps the contents of the ticket data as well.
func DebugTicketDBLiveBucketString(tmdb *stake.TicketDB, bucket uint8,
	verbose bool) (string, error) {
	var buffer bytes.Buffer

	str := fmt.Sprintf("Contents of live ticket bucket %v:\n", bucket)
	buffer.WriteString(str)

	bucketTickets, err := tmdb.DumpLiveTickets(bucket)
	if err != nil {
		return "", err
	}

	for hash, td := range bucketTickets {
		str = fmt.Sprintf("%v\n", hash)
		buffer.WriteString(str)

		if verbose {
			str = fmt.Sprintf("%v\n", DebugTicketDataString(td))
			buffer.WriteString(str)
		}
	}

	return buffer.String(), nil
}

// DebugTicketDBSpentBucketString prints the contents of the spent tickets
// database bucket indicated to a string that is returned. If the verbose
// flag is indicated, the contents of each ticket are printed as well.
func DebugTicketDBSpentBucketString(tmdb *stake.TicketDB, height int64,
	verbose bool) (string, error) {
	var buffer bytes.Buffer

	str := fmt.Sprintf("Contents of spent ticket bucket height %v:\n", height)
	buffer.WriteString(str)

	bucketTickets, err := tmdb.DumpSpentTickets(height)
	if err != nil {
		return "", err
	}

	for hash, td := range bucketTickets {
		missedStr := ""
		if td.Missed {
			missedStr = "Missed"
		} else {
			missedStr = "Spent"
		}
		str = fmt.Sprintf("%v (%v)\n", hash, missedStr)
		buffer.WriteString(str)

		if verbose {
			str = fmt.Sprintf("%v\n", DebugTicketDataString(td))
			buffer.WriteString(str)
		}
	}

	return buffer.String(), nil
}

// DebugTicketDBMissedString prints out the contents of the missed ticket
// database to a string. If verbose is indicated, the ticket data itself
// is printed along with the ticket hashes.
func DebugTicketDBMissedString(tmdb *stake.TicketDB, verbose bool) (string,
	error) {
	var buffer bytes.Buffer

	str := fmt.Sprintf("Contents of missed ticket database:\n")
	buffer.WriteString(str)

	bucketTickets, err := tmdb.DumpMissedTickets()
	if err != nil {
		return "", err
	}

	for hash, td := range bucketTickets {
		str = fmt.Sprintf("%v\n", hash)
		buffer.WriteString(str)

		if verbose {
			str = fmt.Sprintf("%v\n", DebugTicketDataString(td))
			buffer.WriteString(str)
		}
	}

	return buffer.String(), nil
}

// writeTicketDataToBuf writes some ticket data into a buffer as serialized
// data.
func writeTicketDataToBuf(buf *bytes.Buffer, td *stake.TicketData) {
	buf.Write(td.SStxHash[:])
	buf.Write(td.SpendHash[:])

	// OK for our purposes.
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(td.BlockHeight))
	buf.Write(b)

	buf.Write([]byte{byte(td.Prefix)})

	if td.Missed {
		buf.Write([]byte{0x01})
	} else {
		buf.Write([]byte{0x00})
	}

	if td.Expired {
		buf.Write([]byte{0x01})
	} else {
		buf.Write([]byte{0x00})
	}
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

// TicketDbThumbprint takes all the tickets in the respective ticket db,
// sorts them, hashes their contents into a list, and then hashes that list.
// The resultant hash is the thumbprint of the ticket database, and should
// be the same across all clients that are synced to the same block. Returns
// an array of hashes len 3, containing (1) live tickets (2) spent tickets
// and (3) missed tickets.
// Do NOT use on mainnet or in production. For debug use only! Make sure
// the blockchain is frozen when you call this function.
func TicketDbThumbprint(tmdb *stake.TicketDB,
	chainParams *chaincfg.Params) ([]*chainhash.Hash, error) {
	// Container for the three master hashes to go into.
	dbThumbprints := make([]*chainhash.Hash, 3, 3)

	// (1) Live tickets.
	allLiveTickets := stake.NewTicketDataSliceEmpty()
	for i := 0; i < stake.BucketsSize; i++ {
		bucketTickets, err := tmdb.DumpLiveTickets(uint8(i))
		if err != nil {
			return nil, err
		}

		for _, td := range bucketTickets {
			allLiveTickets = append(allLiveTickets, td)
		}
	}

	// Sort by the number data hash, since we already have this implemented
	// and it's also unique.
	sort.Sort(allLiveTickets)

	// Create a buffer, dump all the data into it, and hash.
	var buf bytes.Buffer
	for _, td := range allLiveTickets {
		writeTicketDataToBuf(&buf, td)
	}

	liveHash := chainhash.HashFunc(buf.Bytes())
	liveThumbprint, err := chainhash.NewHash(liveHash[:])
	if err != nil {
		return nil, err
	}
	dbThumbprints[0] = liveThumbprint

	// (2) Spent tickets.
	height := tmdb.GetTopBlock()

	allSpentTickets := stake.NewTicketDataSliceEmpty()
	for i := int64(chainParams.StakeEnabledHeight); i <= height; i++ {
		bucketTickets, err := tmdb.DumpSpentTickets(i)
		if err != nil {
			return nil, err
		}

		for _, td := range bucketTickets {
			allSpentTickets = append(allSpentTickets, td)
		}
	}

	sort.Sort(allSpentTickets)

	buf.Reset() // Flush buffer
	for _, td := range allSpentTickets {
		writeTicketDataToBuf(&buf, td)
	}

	spentHash := chainhash.HashFunc(buf.Bytes())
	spentThumbprint, err := chainhash.NewHash(spentHash[:])
	if err != nil {
		return nil, err
	}
	dbThumbprints[1] = spentThumbprint

	// (3) Missed tickets.
	allMissedTickets := stake.NewTicketDataSliceEmpty()
	missedTickets, err := tmdb.DumpMissedTickets()
	if err != nil {
		return nil, err
	}

	for _, td := range missedTickets {
		allMissedTickets = append(allMissedTickets, td)
	}

	sort.Sort(allMissedTickets)

	buf.Reset() // Flush buffer
	missedHash := chainhash.HashFunc(buf.Bytes())
	missedThumbprint, err := chainhash.NewHash(missedHash[:])
	if err != nil {
		return nil, err
	}
	dbThumbprints[2] = missedThumbprint

	return dbThumbprints, nil
}

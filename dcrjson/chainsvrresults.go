// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import "encoding/json"

// TxRawDecodeResult models the data from the decoderawtransaction command.
type TxRawDecodeResult struct {
	Txid     string `json:"txid"`
	Version  int32  `json:"version"`
	Locktime uint32 `json:"locktime"`
	Expiry   uint32 `json:"expiry"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
}

// DecodeScriptResult models the data returned from the decodescript command.
type DecodeScriptResult struct {
	Asm       string   `json:"asm"`
	ReqSigs   int32    `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
	P2sh      string   `json:"p2sh,omitempty"`
}

// EstimateSmartFeeResult models the data returned from the estimatesmartfee
// command.
type EstimateSmartFeeResult struct {
	FeeRate float64  `json:"feerate"`
	Errors  []string `json:"errors"`
	Blocks  int64    `json:"blocks"`
}

// EstimateStakeDiffResult models the data returned from the estimatestakediff
// command.
type EstimateStakeDiffResult struct {
	Min      float64  `json:"min"`
	Max      float64  `json:"max"`
	Expected float64  `json:"expected"`
	User     *float64 `json:"user,omitempty"`
}

// GetAddedNodeInfoResultAddr models the data of the addresses portion of the
// getaddednodeinfo command.
type GetAddedNodeInfoResultAddr struct {
	Address   string `json:"address"`
	Connected string `json:"connected"`
}

// GetAddedNodeInfoResult models the data from the getaddednodeinfo command.
type GetAddedNodeInfoResult struct {
	AddedNode string                        `json:"addednode"`
	Connected *bool                         `json:"connected,omitempty"`
	Addresses *[]GetAddedNodeInfoResultAddr `json:"addresses,omitempty"`
}

// GetBlockVerboseResult models the data from the getblock command when the
// verbose flag is set.  When the verbose flag is not set, getblock returns a
// hex-encoded string.  Contains Decred additions.
type GetBlockVerboseResult struct {
	Hash          string        `json:"hash"`
	Confirmations int64         `json:"confirmations"`
	Size          int32         `json:"size"`
	Height        int64         `json:"height"`
	Version       int32         `json:"version"`
	MerkleRoot    string        `json:"merkleroot"`
	StakeRoot     string        `json:"stakeroot"`
	Tx            []string      `json:"tx,omitempty"`
	RawTx         []TxRawResult `json:"rawtx,omitempty"`
	STx           []string      `json:"stx,omitempty"`
	RawSTx        []TxRawResult `json:"rawstx,omitempty"`
	Time          int64         `json:"time"`
	Nonce         uint32        `json:"nonce"`
	VoteBits      uint16        `json:"votebits"`
	FinalState    string        `json:"finalstate"`
	Voters        uint16        `json:"voters"`
	FreshStake    uint8         `json:"freshstake"`
	Revocations   uint8         `json:"revocations"`
	PoolSize      uint32        `json:"poolsize"`
	Bits          string        `json:"bits"`
	SBits         float64       `json:"sbits"`
	Difficulty    float64       `json:"difficulty"`
	ExtraData     string        `json:"extradata"`
	StakeVersion  uint32        `json:"stakeversion"`
	PreviousHash  string        `json:"previousblockhash"`
	NextHash      string        `json:"nextblockhash,omitempty"`
}

// GetBlockChainInfoResult models the data returned from the getblockchaininfo
// command.
type GetBlockChainInfoResult struct {
	Chain                string  `json:"chain"`
	Blocks               int32   `json:"blocks"`
	Headers              int32   `json:"headers"`
	BestBlockHash        string  `json:"bestblockhash"`
	Difficulty           float64 `json:"difficulty"`
	VerificationProgress float64 `json:"verificationprogress"`
	ChainWork            string  `json:"chainwork"`
}

// GetBlockHeaderVerboseResult models the data from the getblockheader command when
// the verbose flag is set.  When the verbose flag is not set, getblockheader
// returns a hex-encoded string.
type GetBlockHeaderVerboseResult struct {
	Hash          string  `json:"hash"`
	Confirmations int64   `json:"confirmations"`
	Version       int32   `json:"version"`
	PreviousHash  string  `json:"previousblockhash,omitempty"`
	MerkleRoot    string  `json:"merkleroot"`
	StakeRoot     string  `json:"stakeroot"`
	VoteBits      uint16  `json:"votebits"`
	FinalState    string  `json:"finalstate"`
	Voters        uint16  `json:"voters"`
	FreshStake    uint8   `json:"freshstake"`
	Revocations   uint8   `json:"revocations"`
	PoolSize      uint32  `json:"poolsize"`
	Bits          string  `json:"bits"`
	SBits         float64 `json:"sbits"`
	Height        uint32  `json:"height"`
	Size          uint32  `json:"size"`
	Time          int64   `json:"time"`
	Nonce         uint32  `json:"nonce"`
	StakeVersion  uint32  `json:"stakeversion"`
	Difficulty    float64 `json:"difficulty"`
	NextHash      string  `json:"nextblockhash,omitempty"`
}

// GetBlockSubsidyResult models the data returned from the getblocksubsidy
// command.
type GetBlockSubsidyResult struct {
	Developer int64 `json:"developer"`
	PoS       int64 `json:"pos"`
	PoW       int64 `json:"pow"`
	Total     int64 `json:"total"`
}

// GetBlockTemplateResultTx models the transactions field of the
// getblocktemplate command.
type GetBlockTemplateResultTx struct {
	Data    string  `json:"data"`
	Hash    string  `json:"hash"`
	Depends []int64 `json:"depends"`
	Fee     int64   `json:"fee"`
	SigOps  int64   `json:"sigops"`
	TxType  string  `json:"txtype"`
}

// GetBlockTemplateResultAux models the coinbaseaux field of the
// getblocktemplate command.
type GetBlockTemplateResultAux struct {
	Flags string `json:"flags"`
}

// GetBlockTemplateResult models the data returned from the getblocktemplate
// command.
type GetBlockTemplateResult struct {
	// Base fields from BIP 0022.  CoinbaseAux is optional.  One of
	// CoinbaseTxn or CoinbaseValue must be specified, but not both.
	// GBT has been modified from the Bitcoin semantics to include
	// the header rather than various components which are all part
	// of the header anyway.
	Header        string                     `json:"header"`
	SigOpLimit    int64                      `json:"sigoplimit,omitempty"`
	SizeLimit     int64                      `json:"sizelimit,omitempty"`
	Transactions  []GetBlockTemplateResultTx `json:"transactions"`
	STransactions []GetBlockTemplateResultTx `json:"stransactions"`
	CoinbaseAux   *GetBlockTemplateResultAux `json:"coinbaseaux,omitempty"`
	CoinbaseTxn   *GetBlockTemplateResultTx  `json:"coinbasetxn,omitempty"`
	CoinbaseValue *int64                     `json:"coinbasevalue,omitempty"`
	WorkID        string                     `json:"workid,omitempty"`

	// Optional long polling from BIP 0022.
	LongPollID  string `json:"longpollid,omitempty"`
	LongPollURI string `json:"longpolluri,omitempty"`
	SubmitOld   *bool  `json:"submitold,omitempty"`

	// Basic pool extension from BIP 0023.
	Target  string `json:"target,omitempty"`
	Expires int64  `json:"expires,omitempty"`

	// Mutations from BIP 0023.
	MaxTime    int64    `json:"maxtime,omitempty"`
	MinTime    int64    `json:"mintime,omitempty"`
	Mutable    []string `json:"mutable,omitempty"`
	NonceRange string   `json:"noncerange,omitempty"`

	// Block proposal from BIP 0023.
	Capabilities  []string `json:"capabilities,omitempty"`
	RejectReasion string   `json:"reject-reason,omitempty"`
}

// GetChainTipsResult models the data returns from the getchaintips command.
type GetChainTipsResult struct {
	Height    int64  `json:"height"`
	Hash      string `json:"hash"`
	BranchLen int64  `json:"branchlen"`
	Status    string `json:"status"`
}

// GetHeadersResult models the data returned by the chain server getheaders
// command.
type GetHeadersResult struct {
	Headers []string `json:"headers"`
}

// InfoChainResult models the data returned by the chain server getinfo command.
type InfoChainResult struct {
	Version         int32   `json:"version"`
	ProtocolVersion int32   `json:"protocolversion"`
	Blocks          int64   `json:"blocks"`
	TimeOffset      int64   `json:"timeoffset"`
	Connections     int32   `json:"connections"`
	Proxy           string  `json:"proxy"`
	Difficulty      float64 `json:"difficulty"`
	TestNet         bool    `json:"testnet"`
	RelayFee        float64 `json:"relayfee"`
	Errors          string  `json:"errors"`
}

// GetMempoolInfoResult models the data returned from the getmempoolinfo
// command.
type GetMempoolInfoResult struct {
	Size  int64 `json:"size"`
	Bytes int64 `json:"bytes"`
}

// GetMiningInfoResult models the data from the getmininginfo command.
// Contains Decred additions.
type GetMiningInfoResult struct {
	Blocks           int64   `json:"blocks"`
	CurrentBlockSize uint64  `json:"currentblocksize"`
	CurrentBlockTx   uint64  `json:"currentblocktx"`
	Difficulty       float64 `json:"difficulty"`
	StakeDifficulty  int64   `json:"stakedifficulty"`
	Errors           string  `json:"errors"`
	Generate         bool    `json:"generate"`
	GenProcLimit     int32   `json:"genproclimit"`
	HashesPerSec     int64   `json:"hashespersec"`
	NetworkHashPS    int64   `json:"networkhashps"`
	PooledTx         uint64  `json:"pooledtx"`
	TestNet          bool    `json:"testnet"`
}

// LocalAddressesResult models the localaddresses data from the getnetworkinfo
// command.
type LocalAddressesResult struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
	Score   int32  `json:"score"`
}

// NetworksResult models the networks data from the getnetworkinfo command.
type NetworksResult struct {
	Name      string `json:"name"`
	Limited   bool   `json:"limited"`
	Reachable bool   `json:"reachable"`
	Proxy     string `json:"proxy"`
}

// GetNetworkInfoResult models the data returned from the getnetworkinfo
// command.
type GetNetworkInfoResult struct {
	Version         int32                  `json:"version"`
	ProtocolVersion int32                  `json:"protocolversion"`
	TimeOffset      int64                  `json:"timeoffset"`
	Connections     int32                  `json:"connections"`
	Networks        []NetworksResult       `json:"networks"`
	RelayFee        float64                `json:"relayfee"`
	LocalAddresses  []LocalAddressesResult `json:"localaddresses"`
}

// GetNetTotalsResult models the data returned from the getnettotals command.
type GetNetTotalsResult struct {
	TotalBytesRecv uint64 `json:"totalbytesrecv"`
	TotalBytesSent uint64 `json:"totalbytessent"`
	TimeMillis     int64  `json:"timemillis"`
}

// GetPeerInfoResult models the data returned from the getpeerinfo command.
type GetPeerInfoResult struct {
	ID             int32   `json:"id"`
	Addr           string  `json:"addr"`
	AddrLocal      string  `json:"addrlocal,omitempty"`
	Services       string  `json:"services"`
	RelayTxes      bool    `json:"relaytxes"`
	LastSend       int64   `json:"lastsend"`
	LastRecv       int64   `json:"lastrecv"`
	BytesSent      uint64  `json:"bytessent"`
	BytesRecv      uint64  `json:"bytesrecv"`
	ConnTime       int64   `json:"conntime"`
	TimeOffset     int64   `json:"timeoffset"`
	PingTime       float64 `json:"pingtime"`
	PingWait       float64 `json:"pingwait,omitempty"`
	Version        uint32  `json:"version"`
	SubVer         string  `json:"subver"`
	Inbound        bool    `json:"inbound"`
	StartingHeight int64   `json:"startingheight"`
	CurrentHeight  int64   `json:"currentheight,omitempty"`
	BanScore       int32   `json:"banscore"`
	SyncNode       bool    `json:"syncnode"`
}

// GetRawMempoolVerboseResult models the data returned from the getrawmempool
// command when the verbose flag is set.  When the verbose flag is not set,
// getrawmempool returns an array of transaction hashes.
type GetRawMempoolVerboseResult struct {
	Size             int32    `json:"size"`
	Fee              float64  `json:"fee"`
	Time             int64    `json:"time"`
	Height           int64    `json:"height"`
	StartingPriority float64  `json:"startingpriority"`
	CurrentPriority  float64  `json:"currentpriority"`
	Depends          []string `json:"depends"`
}

// TxRawResult models the data from the getrawtransaction command.
type TxRawResult struct {
	Hex           string `json:"hex"`
	Txid          string `json:"txid"`
	Version       int32  `json:"version"`
	LockTime      uint32 `json:"locktime"`
	Expiry        uint32 `json:"expiry"`
	Vin           []Vin  `json:"vin"`
	Vout          []Vout `json:"vout"`
	BlockHash     string `json:"blockhash,omitempty"`
	BlockHeight   int64  `json:"blockheight"`
	BlockIndex    uint32 `json:"blockindex,omitempty"`
	Confirmations int64  `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

// GetStakeDifficultyResult models the data returned from the
// getstakedifficulty command.
type GetStakeDifficultyResult struct {
	CurrentStakeDifficulty float64 `json:"current"`
	NextStakeDifficulty    float64 `json:"next"`
}

// VersionCount models a generic version:count tuple.
type VersionCount struct {
	Version uint32 `json:"version"`
	Count   uint32 `json:"count"`
}

// VersionInterval models a cooked version count for an interval.
type VersionInterval struct {
	StartHeight  int64          `json:"startheight"`
	EndHeight    int64          `json:"endheight"`
	PoSVersions  []VersionCount `json:"posversions"`
	VoteVersions []VersionCount `json:"voteversions"`
}

// GetStakeVersionInfoResult models the resulting data for getstakeversioninfo
// command.
type GetStakeVersionInfoResult struct {
	CurrentHeight int64             `json:"currentheight"`
	Hash          string            `json:"hash"`
	Intervals     []VersionInterval `json:"intervals"`
}

// VersionBits models a generic version:bits tuple.
type VersionBits struct {
	Version uint32 `json:"version"`
	Bits    uint16 `json:"bits"`
}

// StakeVersions models the data for GetStakeVersionsResult.
type StakeVersions struct {
	Hash         string        `json:"hash"`
	Height       int64         `json:"height"`
	BlockVersion int32         `json:"blockversion"`
	StakeVersion uint32        `json:"stakeversion"`
	Votes        []VersionBits `json:"votes"`
}

// GetStakeVersionsResult models the data returned from the getstakeversions
// command.
type GetStakeVersionsResult struct {
	StakeVersions []StakeVersions `json:"stakeversions"`
}

// GetTxOutResult models the data from the gettxout command.
type GetTxOutResult struct {
	BestBlock     string             `json:"bestblock"`
	Confirmations int64              `json:"confirmations"`
	Value         float64            `json:"value"`
	ScriptPubKey  ScriptPubKeyResult `json:"scriptPubKey"`
	Version       int32              `json:"version"`
	Coinbase      bool               `json:"coinbase"`
}

// Choice models an individual choice inside an Agenda.
type Choice struct {
	Id          string  `json:"id"`
	Description string  `json:"description"`
	Bits        uint16  `json:"bits"`
	IsAbstain   bool    `json:"isabstain"`
	IsNo        bool    `json:"isno"`
	Count       uint32  `json:"count"`
	Progress    float64 `json:"progress"`
}

// Agenda models an individual agenda including its choices.
type Agenda struct {
	Id             string   `json:"id"`
	Description    string   `json:"description"`
	Mask           uint16   `json:"mask"`
	StartTime      uint64   `json:"starttime"`
	ExpireTime     uint64   `json:"expiretime"`
	Status         string   `json:"status"`
	QuorumProgress float64  `json:"quorumprogress"`
	Choices        []Choice `json:"choices"`
}

// GetVoteInfoResult models the data returned from the getvoteinfo command.
type GetVoteInfoResult struct {
	CurrentHeight int64    `json:"currentheight"`
	StartHeight   int64    `json:"startheight"`
	EndHeight     int64    `json:"endheight"`
	Hash          string   `json:"hash"`
	VoteVersion   uint32   `json:"voteversion"`
	Quorum        uint32   `json:"quorum"`
	TotalVotes    uint32   `json:"totalvotes"`
	Agendas       []Agenda `json:"agendas,omitempty"`
}

// GetWorkResult models the data from the getwork command.
type GetWorkResult struct {
	Data   string `json:"data"`
	Target string `json:"target"`
}

// Ticket is the structure representing a ticket.
type Ticket struct {
	Hash  string `json:"hash"`
	Owner string `json:"owner"`
}

// LiveTicketsResult models the data returned from the livetickets
// command.
type LiveTicketsResult struct {
	Tickets []string `json:"tickets"`
}

// MissedTicketsResult models the data returned from the missedtickets
// command.
type MissedTicketsResult struct {
	Tickets []string `json:"tickets"`
}

// FeeInfoBlock is ticket fee information about a block.
type FeeInfoBlock struct {
	Height uint32  `json:"height"`
	Number uint32  `json:"number"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
}

// FeeInfoMempool is ticket fee information about the mempool.
type FeeInfoMempool struct {
	Number uint32  `json:"number"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
}

// FeeInfoRange is ticket fee information about a range.
type FeeInfoRange struct {
	Number uint32  `json:"number"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
}

// FeeInfoWindow is ticket fee information about an adjustment window.
type FeeInfoWindow struct {
	StartHeight uint32  `json:"startheight"`
	EndHeight   uint32  `json:"endheight"`
	Number      uint32  `json:"number"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Mean        float64 `json:"mean"`
	Median      float64 `json:"median"`
	StdDev      float64 `json:"stddev"`
}

// TicketFeeInfoResult models the data returned from the ticketfeeinfo command.
// command.
type TicketFeeInfoResult struct {
	FeeInfoMempool FeeInfoMempool  `json:"feeinfomempool"`
	FeeInfoBlocks  []FeeInfoBlock  `json:"feeinfoblocks"`
	FeeInfoWindows []FeeInfoWindow `json:"feeinfowindows"`
}

// SearchRawTransactionsResult models the data from the searchrawtransaction
// command.
type SearchRawTransactionsResult struct {
	Hex           string       `json:"hex,omitempty"`
	Txid          string       `json:"txid"`
	Version       int32        `json:"version"`
	LockTime      uint32       `json:"locktime"`
	Vin           []VinPrevOut `json:"vin"`
	Vout          []Vout       `json:"vout"`
	BlockHash     string       `json:"blockhash,omitempty"`
	Confirmations uint64       `json:"confirmations,omitempty"`
	Time          int64        `json:"time,omitempty"`
	Blocktime     int64        `json:"blocktime,omitempty"`
}

// TxFeeInfoResult models the data returned from the ticketfeeinfo command.
// command.
type TxFeeInfoResult struct {
	FeeInfoMempool FeeInfoMempool `json:"feeinfomempool"`
	FeeInfoBlocks  []FeeInfoBlock `json:"feeinfoblocks"`
	FeeInfoRange   FeeInfoRange   `json:"feeinforange"`
}

// TicketsForAddressResult models the data returned from the ticketforaddress
// command.
type TicketsForAddressResult struct {
	Tickets []string `json:"tickets"`
}

// ValidateAddressChainResult models the data returned by the chain server
// validateaddress command.
type ValidateAddressChainResult struct {
	IsValid bool   `json:"isvalid"`
	Address string `json:"address,omitempty"`
}

// VersionResult models objects included in the version response.  In the actual
// result, these objects are keyed by the program or API name.
type VersionResult struct {
	VersionString string `json:"versionstring"`
	Major         uint32 `json:"major"`
	Minor         uint32 `json:"minor"`
	Patch         uint32 `json:"patch"`
	Prerelease    string `json:"prerelease"`
	BuildMetadata string `json:"buildmetadata"`
}

// ScriptPubKeyResult models the scriptPubKey data of a tx script.  It is
// defined separately since it is used by multiple commands.
type ScriptPubKeyResult struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex,omitempty"`
	ReqSigs   int32    `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
	CommitAmt *float64 `json:"commitamt,omitempty"`
}

// ScriptSig models a signature script.  It is defined separately since it only
// applies to non-coinbase.  Therefore the field in the Vin structure needs
// to be a pointer.
type ScriptSig struct {
	Asm string `json:"asm"`
	Hex string `json:"hex"`
}

// Vin models parts of the tx data.  It is defined separately since
// getrawtransaction, decoderawtransaction, and searchrawtransaction use the
// same structure.
type Vin struct {
	Coinbase    string     `json:"coinbase"`
	Stakebase   string     `json:"stakebase"`
	Txid        string     `json:"txid"`
	Vout        uint32     `json:"vout"`
	Tree        int8       `json:"tree"`
	Sequence    uint32     `json:"sequence"`
	AmountIn    float64    `json:"amountin"`
	BlockHeight uint32     `json:"blockheight"`
	BlockIndex  uint32     `json:"blockindex"`
	ScriptSig   *ScriptSig `json:"scriptSig"`
}

// IsCoinBase returns a bool to show if a Vin is a Coinbase one or not.
func (v *Vin) IsCoinBase() bool {
	return len(v.Coinbase) > 0
}

// IsStakebase returns a bool to show if a Vin is a StakeBase one or not.
func (v *Vin) IsStakeBase() bool {
	return len(v.Stakebase) > 0
}

// MarshalJSON provides a custom Marshal method for Vin.
func (v *Vin) MarshalJSON() ([]byte, error) {
	if v.IsCoinBase() {
		coinbaseStruct := struct {
			AmountIn    float64 `json:"amountin"`
			BlockHeight uint32  `json:"blockheight"`
			BlockIndex  uint32  `json:"blockindex"`
			Coinbase    string  `json:"coinbase"`
			Sequence    uint32  `json:"sequence"`
		}{
			AmountIn:    v.AmountIn,
			BlockHeight: v.BlockHeight,
			BlockIndex:  v.BlockIndex,
			Coinbase:    v.Coinbase,
			Sequence:    v.Sequence,
		}
		return json.Marshal(coinbaseStruct)
	}

	if v.IsStakeBase() {
		stakebaseStruct := struct {
			AmountIn    float64 `json:"amountin"`
			BlockHeight uint32  `json:"blockheight"`
			BlockIndex  uint32  `json:"blockindex"`
			Stakebase   string  `json:"stakebase"`
			Sequence    uint32  `json:"sequence"`
		}{
			AmountIn:    v.AmountIn,
			BlockHeight: v.BlockHeight,
			BlockIndex:  v.BlockIndex,
			Stakebase:   v.Stakebase,
			Sequence:    v.Sequence,
		}
		return json.Marshal(stakebaseStruct)
	}

	txStruct := struct {
		Txid        string     `json:"txid"`
		Vout        uint32     `json:"vout"`
		Tree        int8       `json:"tree"`
		Sequence    uint32     `json:"sequence"`
		AmountIn    float64    `json:"amountin"`
		BlockHeight uint32     `json:"blockheight"`
		BlockIndex  uint32     `json:"blockindex"`
		ScriptSig   *ScriptSig `json:"scriptSig"`
	}{
		Txid:        v.Txid,
		Vout:        v.Vout,
		Tree:        v.Tree,
		Sequence:    v.Sequence,
		AmountIn:    v.AmountIn,
		BlockHeight: v.BlockHeight,
		BlockIndex:  v.BlockIndex,
		ScriptSig:   v.ScriptSig,
	}
	return json.Marshal(txStruct)
}

// PrevOut represents previous output for an input Vin.
type PrevOut struct {
	Addresses []string `json:"addresses,omitempty"`
	Value     float64  `json:"value"`
}

// VinPrevOut is like Vin except it includes PrevOut.  It is used by searchrawtransaction
type VinPrevOut struct {
	Coinbase    string     `json:"coinbase"`
	Stakebase   string     `json:"stakebase"`
	Txid        string     `json:"txid"`
	Vout        uint32     `json:"vout"`
	Tree        int8       `json:"tree"`
	AmountIn    *float64   `json:"amountin,omitempty"`
	BlockHeight *uint32    `json:"blockheight,omitempty"`
	BlockIndex  *uint32    `json:"blockindex,omitempty"`
	ScriptSig   *ScriptSig `json:"scriptSig"`
	PrevOut     *PrevOut   `json:"prevOut"`
	Sequence    uint32     `json:"sequence"`
}

// IsCoinBase returns a bool to show if a Vin is a Coinbase one or not.
func (v *VinPrevOut) IsCoinBase() bool {
	return len(v.Coinbase) > 0
}

// IsStakebase returns a bool to show if a Vin is a StakeBase one or not.
func (v *VinPrevOut) IsStakeBase() bool {
	return len(v.Stakebase) > 0
}

// MarshalJSON provides a custom Marshal method for VinPrevOut.
func (v *VinPrevOut) MarshalJSON() ([]byte, error) {
	if v.IsCoinBase() {
		coinbaseStruct := struct {
			Coinbase string   `json:"coinbase"`
			AmountIn *float64 `json:"amountin,omitempty"`
			Sequence uint32   `json:"sequence"`
		}{
			Coinbase: v.Coinbase,
			AmountIn: v.AmountIn,
			Sequence: v.Sequence,
		}
		return json.Marshal(coinbaseStruct)
	}

	if v.IsStakeBase() {
		stakebaseStruct := struct {
			Stakebase string   `json:"stakebase"`
			AmountIn  *float64 `json:"amountin,omitempty"`
			Sequence  uint32   `json:"sequence"`
		}{
			Stakebase: v.Stakebase,
			AmountIn:  v.AmountIn,
			Sequence:  v.Sequence,
		}
		return json.Marshal(stakebaseStruct)
	}

	txStruct := struct {
		Txid        string     `json:"txid"`
		Vout        uint32     `json:"vout"`
		Tree        int8       `json:"tree"`
		AmountIn    *float64   `json:"amountin,omitempty"`
		BlockHeight *uint32    `json:"blockheight,omitempty"`
		BlockIndex  *uint32    `json:"blockindex,omitempty"`
		ScriptSig   *ScriptSig `json:"scriptSig"`
		PrevOut     *PrevOut   `json:"prevOut,omitempty"`
		Sequence    uint32     `json:"sequence"`
	}{
		Txid:        v.Txid,
		Vout:        v.Vout,
		Tree:        v.Tree,
		AmountIn:    v.AmountIn,
		BlockHeight: v.BlockHeight,
		BlockIndex:  v.BlockIndex,
		ScriptSig:   v.ScriptSig,
		PrevOut:     v.PrevOut,
		Sequence:    v.Sequence,
	}
	return json.Marshal(txStruct)
}

// Vout models parts of the tx data.  It is defined separately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vout struct {
	Value        float64            `json:"value"`
	N            uint32             `json:"n"`
	Version      uint16             `json:"version"`
	ScriptPubKey ScriptPubKeyResult `json:"scriptPubKey"`
}

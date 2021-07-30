package btcjson

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("getchangesinblock", (*GetChangesInBlockCmd)(nil), flags)
	MustRegisterCmd("getclaimsforname", (*GetClaimsForNameCmd)(nil), flags)
	MustRegisterCmd("getclaimsfornamebyid", (*GetClaimsForNameByIDCmd)(nil), flags)
	MustRegisterCmd("getclaimsfornamebybid", (*GetClaimsForNameByBidCmd)(nil), flags)
	MustRegisterCmd("getclaimsfornamebyseq", (*GetClaimsForNameBySeqCmd)(nil), flags)
	MustRegisterCmd("normalize", (*GetNormalizedCmd)(nil), flags)
}

// optional inputs are required to be pointers, but they support things like `jsonrpcdefault:"false"`
// optional inputs have to be at the bottom of the struct
// optional outputs require ",omitempty"
// traditional bitcoin fields are all lowercase

type GetChangesInBlockCmd struct {
	HashOrHeight *string `json:"hashorheight" jsonrpcdefault:""`
}

type GetChangesInBlockResult struct {
	Hash   string   `json:"hash"`
	Height int32    `json:"height"`
	Names  []string `json:"names"`
}

type GetClaimsForNameCmd struct {
	Name          string  `json:"name"`
	HashOrHeight  *string `json:"hashorheight" jsonrpcdefault:""`
	IncludeValues *bool   `json:"includevalues" jsonrpcdefault:"false"`
}

type GetClaimsForNameByIDCmd struct {
	Name            string   `json:"name"`
	PartialClaimIDs []string `json:"partialclaimids"`
	HashOrHeight    *string  `json:"hashorheight" jsonrpcdefault:""`
	IncludeValues   *bool    `json:"includevalues" jsonrpcdefault:"false"`
}

type GetClaimsForNameByBidCmd struct {
	Name          string  `json:"name"`
	Bids          []int32 `json:"bids"`
	HashOrHeight  *string `json:"hashorheight" jsonrpcdefault:""`
	IncludeValues *bool   `json:"includevalues" jsonrpcdefault:"false"`
}

type GetClaimsForNameBySeqCmd struct {
	Name          string  `json:"name"`
	Sequences     []int32 `json:"sequences" jsonrpcusage:"[sequence,...]"`
	HashOrHeight  *string `json:"hashorheight" jsonrpcdefault:""`
	IncludeValues *bool   `json:"includevalues" jsonrpcdefault:"false"`
}

type GetClaimsForNameResult struct {
	Hash               string        `json:"hash"`
	Height             int32         `json:"height"`
	LastTakeoverHeight int32         `json:"lasttakeoverheight"`
	NormalizedName     string        `json:"normalizedname"`
	Claims             []ClaimResult `json:"claims"`
	// UnclaimedSupports []SupportResult `json:"supportswithoutclaim"` how would this work with other constraints?
}

type SupportResult struct {
	TXID          string `json:"txid"`
	N             uint32 `json:"n"`
	Height        int32  `json:"height"`
	ValidAtHeight int32  `json:"validatheight"`
	Amount        int64  `json:"amount"`
	Address       string `json:"address,omitempty"`
	Value         string `json:"value,omitempty"`
}

type ClaimResult struct {
	ClaimID         string          `json:"claimid"`
	TXID            string          `json:"txid"`
	N               uint32          `json:"n"`
	Bid             int32           `json:"bid"`
	Sequence        int32           `json:"sequence"`
	Height          int32           `json:"height"`
	ValidAtHeight   int32           `json:"validatheight"`
	Amount          int64           `json:"amount"`
	EffectiveAmount int64           `json:"effectiveamount"`
	Supports        []SupportResult `json:"supports,omitempty"`
	Address         string          `json:"address,omitempty"`
	Value           string          `json:"value,omitempty"`
}

type GetNormalizedCmd struct {
	Name string `json:"name"`
}

type GetNormalizedResult struct {
	NormalizedName string `json:"normalizedname"`
}

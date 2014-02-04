package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
	"github.com/conformal/go-flags"
	"github.com/davecgh/go-spew/spew"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
)

// conversionHandler is a handler that is used to convert parameters from the
// command line to a specific type.  This is needed since the btcjson API
// expects certain types for various parameters.
type conversionHandler func(string) (interface{}, error)

// displayHandler is a handler that takes an interface and displays it to
// standard out.  It is used by the handler data to type assert replies and
// show them formatted as desired.
type displayHandler func(interface{}) error

// handlerData contains information about how a command should be handled.
type handlerData struct {
	requiredArgs       int
	optionalArgs       int
	displayHandler     displayHandler
	conversionHandlers []conversionHandler
	makeCmd            func([]interface{}) (btcjson.Cmd, error)
	usage              string
}

// Errors used in the various handlers.
var (
	ErrNoData           = errors.New("No data returned.")
	ErrNoDisplayHandler = errors.New("No display handler specified.")
	ErrUsage            = errors.New("btcctl usage") // Real usage is shown.
)

// commandHandlers is a map of commands and associated handler data that is used
// to validate correctness and perform the command.
var commandHandlers = map[string]*handlerData{
	"addnode":               {2, 0, displayJSONDump, nil, makeAddNode, "<ip> <add/remove/onetry>"},
	"createrawtransaction":  {2, 0, displayGeneric, nil, makeCreateRawTransaction, "\"[{\"txid\":\"id\",\"vout\":n},...]\" \"{\"address\":amount,...}\""},
	"debuglevel":            {1, 0, displayGeneric, nil, makeDebugLevel, "<levelspec>"},
	"decoderawtransaction":  {1, 0, displayJSONDump, nil, makeDecodeRawTransaction, "<txhash>"},
	"decodescript":          {1, 0, displayJSONDump, nil, makeDecodeScript, "<hex>"},
	"dumpprivkey":           {1, 0, displayGeneric, nil, makeDumpPrivKey, "<bitcoinaddress>"},
	"getaccount":            {1, 0, displayGeneric, nil, makeGetAccount, "<address>"},
	"getaccountaddress":     {1, 0, displayGeneric, nil, makeGetAccountAddress, "<account>"},
	"getaddednodeinfo":      {1, 1, displayJSONDump, []conversionHandler{toBool, nil}, makeGetAddedNodeInfo, "<dns> [node]"},
	"getaddressesbyaccount": {1, 0, displayJSONDump, nil, makeGetAddressesByAccount, "[account]"},
	"getbalance":            {0, 2, displayGeneric, []conversionHandler{nil, toInt}, makeGetBalance, "[account] [minconf=1]"},
	"getbestblockhash":      {0, 0, displayGeneric, nil, makeGetBestBlockHash, ""},
	"getblock":              {1, 2, displayJSONDump, []conversionHandler{nil, toBool, toBool}, makeGetBlock, "<blockhash>"},
	"getblockcount":         {0, 0, displayGeneric, nil, makeGetBlockCount, ""},
	"getblockhash":          {1, 0, displayGeneric, []conversionHandler{toInt64}, makeGetBlockHash, "<blocknumber>"},
	"getblocktemplate":      {0, 1, displayJSONDump, nil, makeGetBlockTemplate, "[jsonrequestobject]"},
	"getconnectioncount":    {0, 0, displayGeneric, nil, makeGetConnectionCount, ""},
	"getdifficulty":         {0, 0, displayFloat64, nil, makeGetDifficulty, ""},
	"getgenerate":           {0, 0, displayGeneric, nil, makeGetGenerate, ""},
	"gethashespersec":       {0, 0, displayGeneric, nil, makeGetHashesPerSec, ""},
	"getinfo":               {0, 0, displayJSONDump, nil, makeGetInfo, ""},
	"getnettotals":          {0, 0, displayGeneric, nil, makeGetNetTotals, ""},
	"getnewaddress":         {0, 1, displayGeneric, nil, makeGetNewAddress, "[account]"},
	"getpeerinfo":           {0, 0, displayJSONDump, nil, makeGetPeerInfo, ""},
	"getrawchangeaddress":   {0, 0, displayGeneric, nil, makeGetRawChangeAddress, ""},
	"getrawmempool":         {0, 1, displayJSONDump, []conversionHandler{toBool}, makeGetRawMempool, "[verbose=false]"},
	"getrawtransaction":     {1, 1, displayJSONDump, []conversionHandler{nil, toInt}, makeGetRawTransaction, "<txhash> [verbose=0]"},
	"getreceivedbyaccount":  {1, 1, displayGeneric, []conversionHandler{nil, toInt}, makeGetReceivedByAccount, "<account> [minconf=1]"},
	"getreceivedbyaddress":  {1, 1, displayGeneric, []conversionHandler{nil, toInt}, makeGetReceivedByAddress, "<address> [minconf=1]"},
	"gettxoutsetinfo":       {0, 0, displayJSONDump, nil, makeGetTxOutSetInfo, ""},
	"getwork":               {0, 1, displayJSONDump, nil, makeGetWork, "[jsonrequestobject]"},
	"help":                  {0, 1, displayGeneric, nil, makeHelp, "[commandName]"},
	"importprivkey":         {1, 2, displayGeneric, []conversionHandler{nil, nil, toBool}, makeImportPrivKey, "<wifprivkey> [label] [rescan=true]"},
	"keypoolrefill":         {0, 1, displayGeneric, []conversionHandler{toInt}, makeKeyPoolRefill, "[newsize]"},
	"listaccounts":          {0, 1, displayJSONDump, []conversionHandler{toInt}, makeListAccounts, "[minconf=1]"},
	"listaddressgroupings":  {0, 0, displayJSONDump, nil, makeListAddressGroupings, ""},
	"listreceivedbyaccount": {0, 2, displayJSONDump, []conversionHandler{toInt, toBool}, makeListReceivedByAccount, "[minconf] [includeempty]"},
	"listreceivedbyaddress": {0, 2, displayJSONDump, []conversionHandler{toInt, toBool}, makeListReceivedByAddress, "[minconf] [includeempty]"},
	"listlockunspent":       {0, 0, displayJSONDump, nil, makeListLockUnspent, ""},
	"listsinceblock":        {0, 2, displayJSONDump, []conversionHandler{nil, toInt}, makeListSinceBlock, "[blockhash] [minconf=10]"},
	"listtransactions":      {0, 3, displayJSONDump, []conversionHandler{nil, toInt, toInt}, makeListTransactions, "[account] [count=10] [from=0]"},
	"listunspent":           {0, 3, displayJSONDump, []conversionHandler{toInt, toInt, nil}, makeListUnspent, "[minconf=1] [maxconf=9999999] [jsonaddressarray]"},
	"ping":                  {0, 0, displayGeneric, nil, makePing, ""},
	"sendfrom": {3, 3, displayGeneric, []conversionHandler{nil, nil, toSatoshi, toInt, nil, nil},
		makeSendFrom, "<account> <address> <amount> [minconf=1] [comment] [comment-to]"},
	"sendmany":               {2, 2, displayGeneric, []conversionHandler{nil, nil, toInt, nil}, makeSendMany, "<account> <{\"address\":amount,...}> [minconf=1] [comment]"},
	"sendrawtransaction":     {1, 0, displayGeneric, nil, makeSendRawTransaction, "<hextx>"},
	"sendtoaddress":          {2, 2, displayGeneric, []conversionHandler{nil, toSatoshi, nil, nil}, makeSendToAddress, "<address> <amount> [comment] [comment-to]"},
	"settxfee":               {1, 0, displayGeneric, []conversionHandler{toSatoshi}, makeSetTxFee, "<amount>"},
	"stop":                   {0, 0, displayGeneric, nil, makeStop, ""},
	"submitblock":            {1, 1, displayGeneric, nil, makeSubmitBlock, "<hexdata> [jsonparametersobject]"},
	"validateaddress":        {1, 0, displayJSONDump, nil, makeValidateAddress, "<address>"},
	"verifychain":            {0, 2, displayJSONDump, []conversionHandler{toInt, toInt}, makeVerifyChain, "[level] [numblocks]"},
	"verifymessage":          {3, 0, displayGeneric, nil, makeVerifyMessage, "<address> <signature> <message>"},
	"walletlock":             {0, 0, displayGeneric, nil, makeWalletLock, ""},
	"walletpassphrase":       {1, 1, displayGeneric, []conversionHandler{nil, toInt64}, makeWalletPassphrase, "<passphrase> [timeout]"},
	"walletpassphrasechange": {2, 0, displayGeneric, nil, makeWalletPassphraseChange, "<oldpassphrase> <newpassphrase>"},
}

// toSatoshi attempts to convert the passed string to a satoshi amount returned
// as an int64.  It returns the int64 packed into an interface so it can be used
// in the calls which expect interfaces.  An error will be returned if the string
// can't be converted first to a float64.
func toSatoshi(val string) (interface{}, error) {
	idx, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil, err
	}
	return int64(float64(btcutil.SatoshiPerBitcoin) * idx), nil
}

// toInt attempts to convert the passed string to an integer.  It returns the
// integer packed into an interface so it can be used in the calls which expect
// interfaces.  An error will be returned if the string can't be converted to an
// integer.
func toInt(val string) (interface{}, error) {
	idx, err := strconv.Atoi(val)
	if err != nil {
		return nil, err
	}

	return idx, nil
}

// toInt64 attempts to convert the passed string to an int64.  It returns the
// integer packed into an interface so it can be used in the calls which expect
// interfaces.  An error will be returned if the string can't be converted to an
// integer.
func toInt64(val string) (interface{}, error) {
	idx, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return nil, err
	}

	return idx, nil
}

// toBool attempts to convert the passed string to a bool.  It returns the
// bool packed into the empty interface so it can be used in the calls which
// accept interfaces.  An error will be returned if the string can't be
// converted to a bool.
func toBool(val string) (interface{}, error) {
	return strconv.ParseBool(val)
}

// displayGeneric is a displayHandler that simply displays the passed interface
// using fmt.Println.
func displayGeneric(reply interface{}) error {
	fmt.Println(reply)
	return nil
}

// displayFloat64 is a displayHandler that ensures the concrete type of the
// passed interface is a float64 and displays it using fmt.Println.  An error
// is returned if a float64 is not passed.
func displayFloat64(reply interface{}) error {
	if val, ok := reply.(float64); ok {
		fmt.Printf("%f\n", val)
		return nil
	}

	return fmt.Errorf("reply type is not a float64: %v", spew.Sdump(reply))
}

// displaySpewDump is a displayHandler that simply uses spew.Dump to display the
// passed interface.
func displaySpewDump(reply interface{}) error {
	spew.Dump(reply)
	return nil
}

// displayJSONDump is a displayHandler that uses json.Indent to display the
// passed interface.
func displayJSONDump(reply interface{}) error {
	marshaledBytes, err := json.Marshal(reply)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	err = json.Indent(&buf, marshaledBytes, "", "\t")
	if err != nil {
		return err
	}
	fmt.Println(buf.String())
	return nil
}

// makeAddNode generates the cmd structure for addnode commands.
func makeAddNode(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewAddNodeCmd("btcctl", args[0].(string),
		args[1].(string))
}

// makeCreateRawTransaction generates the cmd structure for createrawtransaction
// commands.
func makeCreateRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	// First unmarshal the JSON provided by the parameters into interfaces.
	var iinputs, iamounts interface{}
	err := json.Unmarshal([]byte(args[0].(string)), &iinputs)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(args[1].(string)), &iamounts)
	if err != nil {
		return nil, err
	}

	// Validate and convert the interfaces to concrete types.
	inputs, amounts, err := btcjson.ConvertCreateRawTxParams(iinputs,
		iamounts)
	if err != nil {
		return nil, err
	}

	return btcjson.NewCreateRawTransactionCmd("btcctl", inputs, amounts)
}

// makeDebugLevel generates the cmd structure for debuglevel commands.
func makeDebugLevel(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewDebugLevelCmd("btcctl", args[0].(string))
}

// makeDecodeRawTransaction generates the cmd structure for
// decoderawtransaction commands.
func makeDecodeRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewDecodeRawTransactionCmd("btcctl", args[0].(string))
}

// makeDecodeScript generates the cmd structure for decodescript commands.
func makeDecodeScript(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewDecodeScriptCmd("btcctl", args[0].(string))
}

// makeDumpPrivKey generates the cmd structure for
// dumpprivkey commands.
func makeDumpPrivKey(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewDumpPrivKeyCmd("btcctl", args[0].(string))
}

// makeGetAccount generates the cmd structure for
// getaccount commands.
func makeGetAccount(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetAccountCmd("btcctl", args[0].(string))
}

// makeGetAccountAddress generates the cmd structure for
// getaccountaddress commands.
func makeGetAccountAddress(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetAccountAddressCmd("btcctl", args[0].(string))
}

// makeGetAddedNodeInfo generates the cmd structure for
// getaccountaddress commands.
func makeGetAddedNodeInfo(args []interface{}) (btcjson.Cmd, error) {
	// Create the getaddednodeinfo command with defaults for the optional
	// parameters.
	cmd, err := btcjson.NewGetAddedNodeInfoCmd("btcctl", args[0].(bool))
	if err != nil {
		return nil, err
	}

	// Override the optional parameter if it was specified.
	if len(args) > 1 {
		cmd.Node = args[1].(string)
	}

	return cmd, nil
}

// makeGetAddressesByAccount generates the cmd structure for
// getaddressesbyaccount commands.
func makeGetAddressesByAccount(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetAddressesByAccountCmd("btcctl", args[0].(string))
}

// makeGetBalance generates the cmd structure for
// getbalance commands.
func makeGetBalance(args []interface{}) (btcjson.Cmd, error) {
	optargs := make([]interface{}, 0, 2)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(string))
	}
	if len(args) > 1 {
		optargs = append(optargs, args[1].(int))
	}

	return btcjson.NewGetBalanceCmd("btcctl", optargs...)
}

// makeGetBestBlockHash generates the cmd structure for
// makebestblockhash commands.
func makeGetBestBlockHash(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetBestBlockHashCmd("btcctl")
}

// makeGetBlock generates the cmd structure for getblock commands.
func makeGetBlock(args []interface{}) (btcjson.Cmd, error) {
	// Create the getblock command with defaults for the optional
	// parameters.
	getBlockCmd, err := btcjson.NewGetBlockCmd("btcctl", args[0].(string))
	if err != nil {
		return nil, err
	}

	// Override the optional parameters if they were specified.
	if len(args) > 1 {
		getBlockCmd.Verbose = args[1].(bool)
	}
	if len(args) > 2 {
		getBlockCmd.VerboseTx = args[2].(bool)
	}

	return getBlockCmd, nil
}

// makeGetBlockCount generates the cmd structure for getblockcount commands.
func makeGetBlockCount(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetBlockCountCmd("btcctl")
}

// makeGetBlockHash generates the cmd structure for getblockhash commands.
func makeGetBlockHash(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetBlockHashCmd("btcctl", args[0].(int64))
}

// makeGetBlockTemplate generates the cmd structure for getblocktemplate commands.
func makeGetBlockTemplate(args []interface{}) (btcjson.Cmd, error) {
	cmd, err := btcjson.NewGetBlockTemplateCmd("btcctl")
	if err != nil {
		return nil, err
	}
	if len(args) == 1 {
		err = cmd.UnmarshalJSON([]byte(args[0].(string)))
		if err != nil {
			return nil, err
		}
	}
	return cmd, nil
}

// makeGetConnectionCount generates the cmd structure for
// getconnectioncount commands.
func makeGetConnectionCount(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetConnectionCountCmd("btcctl")
}

// makeGetDifficulty generates the cmd structure for
// getdifficulty commands.
func makeGetDifficulty(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetDifficultyCmd("btcctl")
}

// makeGetGenerate generates the cmd structure for
// getgenerate commands.
func makeGetGenerate(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetGenerateCmd("btcctl")
}

// makeGetHashesPerSec generates the cmd structure for gethashespersec commands.
func makeGetHashesPerSec(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetHashesPerSecCmd("btcctl")
}

// makeGetInfo generates the cmd structure for getinfo commands.
func makeGetInfo(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetInfoCmd("btcctl")
}

// makeGetNetTotals generates the cmd structure for getnettotals commands.
func makeGetNetTotals(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetNetTotalsCmd("btcctl")
}

// makeGetNewAddress generates the cmd structure for getnewaddress commands.
func makeGetNewAddress(args []interface{}) (btcjson.Cmd, error) {
	var account string
	if len(args) > 0 {
		account = args[0].(string)
	}
	return btcjson.NewGetNewAddressCmd("btcctl", account)
}

// makePeerInfo generates the cmd structure for
// getpeerinfo commands.
func makeGetPeerInfo(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetPeerInfoCmd("btcctl")
}

// makeGetRawChangeAddress generates the cmd structure for getrawchangeaddress commands.
func makeGetRawChangeAddress(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetRawChangeAddressCmd("btcctl")
}

// makeRawMempool generates the cmd structure for
// getrawmempool commands.
func makeGetRawMempool(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]bool, 0, 1)
	if len(args) > 0 {
		opt = append(opt, args[0].(bool))
	}
	return btcjson.NewGetRawMempoolCmd("btcctl", opt...)
}

// makeGetReceivedByAccount generates the cmd structure for
// getreceivedbyaccount commands.
func makeGetReceivedByAccount(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]int, 0, 1)
	if len(args) > 1 {
		opt = append(opt, args[1].(int))
	}
	return btcjson.NewGetReceivedByAccountCmd("btcctl", args[0].(string), opt...)
}

// makeGetReceivedByAddress generates the cmd structure for
// getreceivedbyaddress commands.
func makeGetReceivedByAddress(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]int, 0, 1)
	if len(args) > 1 {
		opt = append(opt, args[1].(int))
	}
	return btcjson.NewGetReceivedByAddressCmd("btcctl", args[0].(string), opt...)
}

// makeGetTxOutSetInfo generates the cmd structure for gettxoutsetinfo commands.
func makeGetTxOutSetInfo(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetTxOutSetInfoCmd("btcctl")
}

func makeGetWork(args []interface{}) (btcjson.Cmd, error) {
	cmd, err := btcjson.NewGetWorkCmd("btcctl")
	if err != nil {
		return nil, err
	}
	if len(args) == 1 {
		err = cmd.UnmarshalJSON([]byte(args[0].(string)))
		if err != nil {
			return nil, err
		}
	}
	return cmd, nil
}

func makeHelp(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]string, 0, 1)
	if len(args) > 0 {
		opt = append(opt, args[0].(string))
	}
	return btcjson.NewHelpCmd("btcctl", opt...)
}

// makeRawTransaction generates the cmd structure for
// getrawtransaction commands.
func makeGetRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]int, 0, 1)
	if len(args) > 1 {
		opt = append(opt, args[1].(int))
	}

	return btcjson.NewGetRawTransactionCmd("btcctl", args[0].(string), opt...)
}

// makeImportPrivKey generates the cmd structure for
// importprivkey commands.
func makeImportPrivKey(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 2)
	if len(args) > 1 {
		optargs = append(optargs, args[1].(string))
	}
	if len(args) > 2 {
		optargs = append(optargs, args[2].(bool))
	}

	return btcjson.NewImportPrivKeyCmd("btcctl", args[0].(string), optargs...)
}

// makeKeyPoolRefill generates the cmd structure for keypoolrefill commands.
func makeKeyPoolRefill(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]uint, 0, 1)
	if len(args) > 0 {
		optargs = append(optargs, uint(args[0].(int)))
	}

	return btcjson.NewKeyPoolRefillCmd("btcctl", optargs...)
}

// makeListAccounts generates the cmd structure for listaccounts commands.
func makeListAccounts(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]int, 0, 1)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(int))
	}
	return btcjson.NewListAccountsCmd("btcctl", optargs...)
}

// makeListAddressGroupings generates the cmd structure for listaddressgroupings commands.
func makeListAddressGroupings(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewListAddressGroupingsCmd("btcctl")
}

// makeListReceivedByAccount generates the cmd structure for listreceivedbyaccount commands.
func makeListReceivedByAccount(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 2)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(int))
	}
	if len(args) > 1 {
		optargs = append(optargs, args[1].(bool))
	}
	return btcjson.NewListReceivedByAccountCmd("btcctl", optargs...)
}

// makeListReceivedByAddress generates the cmd structure for listreceivedbyaddress commands.
func makeListReceivedByAddress(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 2)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(int))
	}
	if len(args) > 1 {
		optargs = append(optargs, args[1].(bool))
	}
	return btcjson.NewListReceivedByAddressCmd("btcctl", optargs...)
}

// makeListLockUnspent generates the cmd structure for listlockunspent commands.
func makeListLockUnspent(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewListLockUnspentCmd("btcctl")
}

// makeListSinceBlock generates the cmd structure for
// listsinceblock commands.
func makeListSinceBlock(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 2)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(string))
	}
	if len(args) > 1 {
		optargs = append(optargs, args[1].(int))
	}

	return btcjson.NewListSinceBlockCmd("btcctl", optargs...)
}

// makeListTransactions generates the cmd structure for
// listtransactions commands.
func makeListTransactions(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 3)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(string))
	}
	if len(args) > 1 {
		optargs = append(optargs, args[1].(int))
	}
	if len(args) > 2 {
		optargs = append(optargs, args[2].(int))
	}

	return btcjson.NewListTransactionsCmd("btcctl", optargs...)
}

// makeListUnspent generates the cmd structure for listunspent commands.
func makeListUnspent(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 3)
	if len(args) > 0 {
		optargs = append(optargs, args[0].(int))
	}
	if len(args) > 1 {
		optargs = append(optargs, args[1].(int))
	}
	if len(args) > 2 {
		var addrs []string
		err := json.Unmarshal([]byte(args[2].(string)), &addrs)
		if err != nil {
			return nil, err
		}
		optargs = append(optargs, addrs)
	}
	return btcjson.NewListUnspentCmd("btcctl", optargs...)
}

// makePing generates the cmd structure for ping commands.
func makePing(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewPingCmd("btcctl")
}

// makeSendFrom generates the cmd structure for sendfrom commands.
func makeSendFrom(args []interface{}) (btcjson.Cmd, error) {
	var optargs = make([]interface{}, 0, 3)
	if len(args) > 3 {
		optargs = append(optargs, args[3].(int))
	}
	if len(args) > 4 {
		optargs = append(optargs, args[4].(string))
	}
	if len(args) > 5 {
		optargs = append(optargs, args[5].(string))
	}
	return btcjson.NewSendFromCmd("btcctl", args[0].(string),
		args[1].(string), args[2].(int64), optargs...)
}

// makeSendMany generates the cmd structure for sendmany commands.
func makeSendMany(args []interface{}) (btcjson.Cmd, error) {
	origPairs := make(map[string]float64)
	err := json.Unmarshal([]byte(args[1].(string)), &origPairs)
	if err != nil {
		return nil, err
	}
	pairs := make(map[string]int64)
	for addr, value := range origPairs {
		pairs[addr] = int64(float64(btcutil.SatoshiPerBitcoin) * value)
	}

	var optargs = make([]interface{}, 0, 2)
	if len(args) > 2 {
		optargs = append(optargs, args[2].(int))
	}
	if len(args) > 3 {
		optargs = append(optargs, args[3].(string))
	}
	return btcjson.NewSendManyCmd("btcctl", args[0].(string), pairs, optargs...)
}

// makeSendRawTransaction generates the cmd structure for sendrawtransaction
// commands.
func makeSendRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewSendRawTransactionCmd("btcctl", args[0].(string))
}

// makeSendToAddress generates the cmd struture for sendtoaddress commands.
func makeSendToAddress(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewSendToAddressCmd("btcctl", args[0].(string), args[1].(int64), args[2:]...)
}

// makeSetTxFee generates the cmd structure for settxfee commands.
func makeSetTxFee(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewSetTxFeeCmd("btcctl", args[0].(int64))
}

// makeStop generates the cmd structure for stop commands.
func makeStop(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewStopCmd("btcctl")
}

// makeSubmitBlock generates the cmd structure for submitblock commands.
func makeSubmitBlock(args []interface{}) (btcjson.Cmd, error) {
	opts := &btcjson.SubmitBlockOptions{}
	if len(args) == 2 {
		opts.WorkId = args[1].(string)
	}

	return btcjson.NewSubmitBlockCmd("btcctl", args[0].(string), opts)
}

// makeValidateAddress generates the cmd structure for validateaddress commands.
func makeValidateAddress(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewValidateAddressCmd("btcctl", args[0].(string))
}

// makeVerifyChain generates the cmd structure for verifychain commands.
func makeVerifyChain(args []interface{}) (btcjson.Cmd, error) {
	iargs := make([]int32, 0, 2)
	for _, i := range args {
		iargs = append(iargs, int32(i.(int)))
	}
	return btcjson.NewVerifyChainCmd("btcctl", iargs...)
}

func makeVerifyMessage(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewVerifyMessageCmd("btcctl", args[0].(string),
		args[1].(string), args[2].(string))
}

// makeWalletLock generates the cmd structure for walletlock commands.
func makeWalletLock(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewWalletLockCmd("btcctl")
}

// makeWalletPassphrase generates the cmd structure for walletpassphrase commands.
func makeWalletPassphrase(args []interface{}) (btcjson.Cmd, error) {
	timeout := int64(60)
	if len(args) > 1 {
		timeout = args[1].(int64)
	}
	return btcjson.NewWalletPassphraseCmd("btcctl", args[0].(string), timeout)
}

// makeWalletPassphraseChange generates the cmd structure for
// walletpassphrasechange commands.
func makeWalletPassphraseChange(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewWalletPassphraseChangeCmd("btcctl", args[0].(string),
		args[1].(string))
}

// send sends a JSON-RPC command to the specified RPC server and examines the
// results for various error conditions.  It either returns a valid result or
// an appropriate error.
func send(cfg *config, msg []byte) (interface{}, error) {
	var reply btcjson.Reply
	var err error
	if cfg.NoTls || (cfg.RPCCert == "" && !cfg.TlsSkipVerify) {
		reply, err = btcjson.RpcCommand(cfg.RPCUser, cfg.RPCPassword,
			cfg.RPCServer, msg)
	} else {
		var pem []byte
		if cfg.RPCCert != "" {
			pem, err = ioutil.ReadFile(cfg.RPCCert)
			if err != nil {
				return nil, err
			}
		}
		reply, err = btcjson.TlsRpcCommand(cfg.RPCUser,
			cfg.RPCPassword, cfg.RPCServer, msg, pem,
			cfg.TlsSkipVerify)
	}
	if err != nil {
		return nil, err
	}

	if reply.Error != nil {
		return nil, reply.Error
	}

	if reply.Result == nil {
		return nil, ErrNoData
	}

	return reply.Result, nil
}

// sendCommand creates a JSON-RPC command using the passed command and arguments
// and then sends it.  A prefix is added to any errors that occur indicating
// what step failed.
func sendCommand(cfg *config, command btcjson.Cmd) (interface{}, error) {
	msg, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("CreateMessage: %v", err.Error())
	}

	reply, err := send(cfg, msg)
	if err != nil {
		return nil, fmt.Errorf("RpcCommand: %v", err.Error())
	}

	return reply, nil
}

// commandHandler handles commands provided via the cli using the specific
// handler data to instruct the handler what to do.
func commandHandler(cfg *config, command string, data *handlerData, args []string) error {
	// Ensure the number of arguments are the expected value.
	if len(args) < data.requiredArgs {
		return ErrUsage
	}
	if len(args) > data.requiredArgs+data.optionalArgs {
		return ErrUsage
	}

	// Ensure there is a display handler.
	if data.displayHandler == nil {
		return ErrNoDisplayHandler
	}

	// Ensure the number of conversion handlers is valid if any are
	// specified.
	convHandlers := data.conversionHandlers
	if convHandlers != nil && len(convHandlers) < len(args) {
		return fmt.Errorf("The number of conversion handlers is invalid.")
	}

	// Convert input parameters per the conversion handlers.
	iargs := make([]interface{}, len(args))
	for i, arg := range args {
		iargs[i] = arg
	}
	for i := range iargs {
		if convHandlers != nil {
			converter := convHandlers[i]
			if converter != nil {
				convertedArg, err := converter(args[i])
				if err != nil {
					return err
				}
				iargs[i] = convertedArg
			}
		}
	}
	cmd, err := data.makeCmd(iargs)
	if err != nil {
		return err
	}

	// Create and send the appropriate JSON-RPC command.
	reply, err := sendCommand(cfg, cmd)
	if err != nil {
		return err
	}

	// Display the results of the JSON-RPC command using the provided
	// display handler.
	err = data.displayHandler(reply)
	if err != nil {
		return err
	}

	return nil
}

// usage displays the command usage.
func usage(parser *flags.Parser) {
	parser.WriteHelp(os.Stderr)

	// Extract usage information for each command from the command handler
	// data and sort by command name.
	fmt.Fprintf(os.Stderr, "\nCommands:\n")
	usageStrings := make([]string, 0, len(commandHandlers))
	for command, data := range commandHandlers {
		usage := command
		if len(data.usage) > 0 {
			usage += " " + data.usage
		}
		usageStrings = append(usageStrings, usage)
	}
	sort.Sort(sort.StringSlice(usageStrings))
	for _, usage := range usageStrings {
		fmt.Fprintf(os.Stderr, "\t%s\n", usage)
	}
}

func main() {
	parser, cfg, args, err := loadConfig()
	if err != nil {
		usage(parser)
		os.Exit(1)
	}
	if len(args) < 1 {
		usage(parser)
		return
	}

	// Display usage if the command is not supported.
	data, exists := commandHandlers[args[0]]
	if !exists {
		fmt.Fprintf(os.Stderr, "Unrecognized command: %s\n", args[0])
		usage(parser)
		os.Exit(1)
	}

	// Execute the command.
	err = commandHandler(cfg, args[0], data, args[1:])
	if err != nil {
		if err == ErrUsage {
			usage(parser)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

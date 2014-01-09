package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/conformal/btcjson"
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
	"addnode":              &handlerData{2, 0, displayJSONDump, nil, makeAddNode, "<ip> <add/remove/onetry>"},
	"createrawtransaction": &handlerData{2, 0, displayGeneric, nil, makeCreateRawTransaction, "\"[{\"txid\":\"id\",\"vout\":n},...]\" \"{\"address\":amount,...}\""},
	"debuglevel":           &handlerData{1, 0, displayGeneric, nil, makeDebugLevel, "<levelspec>"},
	"decoderawtransaction": &handlerData{1, 0, displayJSONDump, nil, makeDecodeRawTransaction, "<txhash>"},
	"decodescript":         &handlerData{1, 0, displayJSONDump, nil, makeDecodeScript, "<hex>"},
	"dumpprivkey":          &handlerData{1, 0, displayGeneric, nil, makeDumpPrivKey, "<bitcoinaddress>"},
	"getaccount":           &handlerData{1, 0, displayGeneric, nil, makeGetAccount, "<address>"},
	"getaccountaddress":    &handlerData{1, 0, displayGeneric, nil, makeGetAccountAddress, "<account>"},
	"getbalance":           &handlerData{0, 2, displayGeneric, []conversionHandler{nil, toInt}, makeGetBalance, "[account] [minconf=1]"},
	"getbestblockhash":     &handlerData{0, 0, displayGeneric, nil, makeGetBestBlockHash, ""},
	"getblock":             &handlerData{1, 2, displayJSONDump, []conversionHandler{nil, toBool, toBool}, makeGetBlock, "<blockhash>"},
	"getblockcount":        &handlerData{0, 0, displayGeneric, nil, makeGetBlockCount, ""},
	"getblockhash":         &handlerData{1, 0, displayGeneric, []conversionHandler{toInt64}, makeGetBlockHash, "<blocknumber>"},
	"getconnectioncount":   &handlerData{0, 0, displayGeneric, nil, makeGetConnectionCount, ""},
	"getdifficulty":        &handlerData{0, 0, displayFloat64, nil, makeGetDifficulty, ""},
	"getgenerate":          &handlerData{0, 0, displayGeneric, nil, makeGetGenerate, ""},
	"gethashespersec":      &handlerData{0, 0, displayGeneric, nil, makeGetHashesPerSec, ""},
	"getpeerinfo":          &handlerData{0, 0, displayJSONDump, nil, makeGetPeerInfo, ""},
	"getrawmempool":        &handlerData{0, 1, displayJSONDump, []conversionHandler{toBool}, makeGetRawMempool, "[verbose=false]"},
	"getrawtransaction":    &handlerData{1, 1, displayJSONDump, []conversionHandler{nil, toBool}, makeGetRawTransaction, "<txhash> [verbose=false]"},
	"importprivkey":        &handlerData{1, 2, displayGeneric, []conversionHandler{nil, nil, toBool}, makeImportPrivKey, "<wifprivkey> [label] [rescan=true]"},
	"listtransactions":     &handlerData{0, 3, displayJSONDump, []conversionHandler{nil, toInt, toInt}, makeListTransactions, "[account] [count=10] [from=0]"},
	"verifychain":          &handlerData{0, 2, displayJSONDump, []conversionHandler{toInt, toInt}, makeVerifyChain, "[level] [numblocks]"},
	"sendrawtransaction":   &handlerData{1, 0, displayGeneric, nil, makeSendRawTransaction, "<hextx>"},
	"stop":                 &handlerData{0, 0, displayGeneric, nil, makeStop, ""},
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

// makeAddNode generates the cmd structure for addnode comands.
func makeAddNode(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewAddNodeCmd("btcctl", args[0].(string),
		args[1].(string))
}

// makeCreateRawTransaction generates the cmd structure for createrawtransaction
// comands.
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
// decoderawtransaction comands.
func makeDecodeRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewDecodeRawTransactionCmd("btcctl", args[0].(string))
}

// makeDecodeScript generates the cmd structure for decodescript comands.
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
// makebestblockhash comands.
func makeGetBestBlockHash(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetBestBlockHashCmd("btcctl")
}

// makeGetBlock generates the cmd structure for getblock comands.
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

// makeGetBlockCount generates the cmd structure for getblockcount comands.
func makeGetBlockCount(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetBlockCountCmd("btcctl")
}

// makeGetBlockHash generates the cmd structure for getblockhash comands.
func makeGetBlockHash(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetBlockHashCmd("btcctl", args[0].(int64))
}

// makeGetConnectionCount generates the cmd structure for
// getconnectioncount comands.
func makeGetConnectionCount(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetConnectionCountCmd("btcctl")
}

// makeGetDifficulty generates the cmd structure for
// getdifficulty comands.
func makeGetDifficulty(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetDifficultyCmd("btcctl")
}

// makeGetGenerate generates the cmd structure for
// getgenerate comands.
func makeGetGenerate(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetGenerateCmd("btcctl")
}

// makeGetHashesPerSec generates the cmd structure for gethashespersec comands.
func makeGetHashesPerSec(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetHashesPerSecCmd("btcctl")
}

// makePeerInfo generates the cmd structure for
// getpeerinfo comands.
func makeGetPeerInfo(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewGetPeerInfoCmd("btcctl")
}

// makeRawMempool generates the cmd structure for
// getrawmempool comands.
func makeGetRawMempool(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]bool, 0, 1)
	if len(args) > 0 {
		opt = append(opt, args[0].(bool))
	}
	return btcjson.NewGetRawMempoolCmd("btcctl", opt...)
}

// makeRawTransaction generates the cmd structure for
// getrawtransaction comands.
func makeGetRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	opt := make([]bool, 0, 1)
	if len(args) > 1 {
		opt = append(opt, args[1].(bool))
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

// makeSendRawTransaction generates the cmd structure for sendrawtransaction
// commands.
func makeSendRawTransaction(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewSendRawTransactionCmd("btcctl", args[0].(string))
}

// makeStop generates the cmd structure for stop comands.
func makeStop(args []interface{}) (btcjson.Cmd, error) {
	return btcjson.NewStopCmd("btcctl")
}

// makeVerifyChain generates the cmd structure for verifychain comands.
func makeVerifyChain(args []interface{}) (btcjson.Cmd, error) {
	iargs := make([]int32, 0, 2)
	for _, i := range args {
		iargs = append(iargs, int32(i.(int)))
	}
	return btcjson.NewVerifyChainCmd("btcctl", iargs...)
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

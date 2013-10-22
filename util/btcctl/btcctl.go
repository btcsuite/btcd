package main

import (
	"errors"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/conformal/go-flags"
	"github.com/davecgh/go-spew/spew"
	"os"
	"sort"
	"strconv"
)

type config struct {
	Help        bool   `short:"h" long:"help" description:"Help"`
	RpcUser     string `short:"u" description:"RPC username"`
	RpcPassword string `short:"P" long:"rpcpass" description:"RPC password"`
	RpcServer   string `short:"s" long:"rpcserver" description:"RPC server to connect to"`
}

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
	usage              string
}

var (
	ErrNoData           = errors.New("No data returned.")
	ErrNoDisplayHandler = errors.New("No display handler specified.")
	ErrUsage            = errors.New("btcctl usage") // Real usage is shown.
)

// commandHandlers is a map of commands and associated handler data that is used
// to validate correctness and perform the command.
var commandHandlers = map[string]*handlerData{
	"addnode":              &handlerData{2, 0, displaySpewDump, nil, "<ip> <add/remove/onetry>"},
	"decoderawtransaction": &handlerData{1, 0, displaySpewDump, nil, "<txhash>"},
	"getbestblockhash":     &handlerData{0, 0, displayGeneric, nil, ""},
	"getblock":             &handlerData{1, 0, displaySpewDump, nil, "<blockhash>"},
	"getblockcount":        &handlerData{0, 0, displayFloat64, nil, ""},
	"getblockhash":         &handlerData{1, 0, displayGeneric, []conversionHandler{toInt}, "<blocknumber>"},
	"getconnectioncount":   &handlerData{0, 0, displayFloat64, nil, ""},
	"getdifficulty":        &handlerData{0, 0, displayFloat64, nil, ""},
	"getgenerate":          &handlerData{0, 0, displayGeneric, nil, ""},
	"getpeerinfo":          &handlerData{0, 0, displaySpewDump, nil, ""},
	"getrawmempool":        &handlerData{0, 0, displaySpewDump, nil, ""},
	"getrawtransaction":    &handlerData{1, 1, displaySpewDump, []conversionHandler{nil, toInt}, "<txhash> [verbose=0]"},
	"stop":                 &handlerData{0, 0, displayGeneric, nil, ""},
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
		fmt.Println(val)
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

// send sends a JSON-RPC command to the specified RPC server and examines the
// results for various error conditions.  It either returns a valid result or
// an appropriate error.
func send(cfg *config, msg []byte) (interface{}, error) {
	reply, err := btcjson.RpcCommand(cfg.RpcUser, cfg.RpcPassword, cfg.RpcServer, msg)
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
func sendCommand(cfg *config, command string, args ...interface{}) (interface{}, error) {
	msg, err := btcjson.CreateMessage(command, args...)
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

	// Create and send the appropriate JSON-RPC command.
	reply, err := sendCommand(cfg, command, iargs...)
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
	// Parse command line and show usage if needed.
	cfg := config{
		RpcServer: "127.0.0.1:8334",
	}
	parser := flags.NewParser(&cfg, flags.PassDoubleDash)
	args, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			usage(parser)
		}
		os.Exit(1)
	}
	if len(args) < 1 || cfg.Help {
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
	err = commandHandler(&cfg, args[0], data, args[1:])
	if err != nil {
		if err == ErrUsage {
			usage(parser)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

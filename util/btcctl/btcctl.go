package main

import (
	"errors"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/conformal/go-flags"
	"github.com/davecgh/go-spew/spew"
	"os"
	"strconv"
)

type config struct {
	Help        bool   `short:"h" long:"help" description:"Help"`
	RpcUser     string `short:"u" description:"RPC username"`
	RpcPassword string `short:"P" long:"rpcpass" description:"RPC password"`
	RpcServer   string `short:"s" long:"rpcserver" description:"RPC server to connect to"`
}

var (
	ErrNoData = errors.New("No data returned.")
)

func main() {
	cfg := config{
		RpcServer: "127.0.0.1:8334",
	}
	parser := flags.NewParser(&cfg, flags.None)

	args, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			usage(parser)
		}
		return
	}

	if len(args) < 1 || cfg.Help {
		usage(parser)
		return
	}

	switch args[0] {
	default:
		usage(parser)
	case "decoderawtransaction":
		if len(args) != 2 {
			usage(parser)
			break
		}
		msg, err := btcjson.CreateMessage("decoderawtransaction", args[1])
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		spew.Dump(reply)
	case "getbestblockhash":
		msg, err := btcjson.CreateMessage("getbestblockhash")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%s\n", reply.(string))
	case "getblock":
		if len(args) != 2 {
			usage(parser)
			break
		}
		msg, err := btcjson.CreateMessage("getblock", args[1])
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		spew.Dump(reply.(btcjson.BlockResult))
	case "getblockcount":
		msg, err := btcjson.CreateMessage("getblockcount")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%d\n", int(reply.(float64)))
	case "getblockhash":
		if len(args) != 2 {
			usage(parser)
			break
		}
		idx, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Printf("Atoi: %v\n", err)
			break
		}
		msg, err := btcjson.CreateMessage("getblockhash", idx)
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%v\n", reply)
	case "getconnectioncount":
		msg, err := btcjson.CreateMessage("getconnectioncount")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%d\n", int(reply.(float64)))
	case "getdifficulty":
		msg, err := btcjson.CreateMessage("getdifficulty")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%f\n", reply.(float64))
	case "getgenerate":
		msg, err := btcjson.CreateMessage("getgenerate")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%v\n", reply.(bool))
	case "getrawmempool":
		msg, err := btcjson.CreateMessage("getrawmempool")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		spew.Dump(reply)
	case "getrawtransaction":
		if len(args) != 2 {
			usage(parser)
			break
		}
		msg, err := btcjson.CreateMessage("getrawtransaction", args[1], 1)
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		spew.Dump(reply)
	case "stop":
		msg, err := btcjson.CreateMessage("stop")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(&cfg, msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%s\n", reply.(string))
	}
}

func send(cfg *config, msg []byte) (interface{}, error) {
	reply, err := btcjson.RpcCommand(cfg.RpcUser, cfg.RpcPassword, cfg.RpcServer, msg)
	if err != nil {
		return nil, err
	}
	if reply.Error != nil {
		return nil, reply.Error
	}
	if reply.Result == nil {
		err := ErrNoData
		return nil, err
	}
	return reply.Result, nil
}

func usage(parser *flags.Parser) {
	parser.WriteHelp(os.Stderr)
	fmt.Fprintf(os.Stderr,
		"\nCommands:\n"+
			"\tdecoderawtransaction <txhash>\n"+
			"\tgetbestblockhash\n"+
			"\tgetblock <blockhash>\n"+
			"\tgetblockcount\n"+
			"\tgetblockhash <blocknumber>\n"+
			"\tgetconnectioncount\n"+
			"\tgetdifficulty\n"+
			"\tgetgenerate\n"+
			"\tgetrawmempool\n"+
			"\tgetrawtransaction <txhash>\n"+
			"\tstop\n")
}

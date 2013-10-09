package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/davecgh/go-spew/spew"
	"strconv"
)

const (
	User     = "rpcuser"
	Password = "rpcpass"
	Server   = "127.0.0.1:8332"
)

var (
	ErrNoData = errors.New("No data returned.")
)

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		return
	}

	switch args[0] {
	default:
		usage()
	case "getblock":
		if len(args) != 2 {
			usage()
			break
		}
		msg, err := btcjson.CreateMessage("getblock", args[1])
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(msg)
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
		reply, err := send(msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%d\n", int(reply.(float64)))
	case "getblockhash":
		if len(args) != 2 {
			usage()
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
		reply, err := send(msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%v\n", reply)
	case "getgenerate":
		msg, err := btcjson.CreateMessage("getgenerate")
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%v\n", reply.(bool))
	case "getrawtransaction":
		if len(args) != 2 {
			usage()
			break
		}
		msg, err := btcjson.CreateMessage("getrawtransaction", args[1], 1)
		if err != nil {
			fmt.Printf("CreateMessage: %v\n", err)
			break
		}
		reply, err := send(msg)
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
		reply, err := send(msg)
		if err != nil {
			fmt.Printf("RpcCommand: %v\n", err)
			break
		}
		fmt.Printf("%s\n", reply.(string))
	}
}

func send(msg []byte) (interface{}, error) {
	reply, err := btcjson.RpcCommand(User, Password, Server, msg)
	if err != nil {
		return 0, err
	}
	if reply.Result == nil {
		err := ErrNoData
		return 0, err
	}
	return reply.Result, nil
}

func usage() {
	fmt.Printf(
		"usage:\n" +
			"\tgetblock <blockhash>\n" +
			"\tgetblockcount\n" +
			"\tgetblockhash <blocknumber>\n" +
			"\tgetgenerate\n" +
			"\tgetrawtransaction <txhash>\n" +
			"\tstop\n")
}

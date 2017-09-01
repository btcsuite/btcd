package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/decred/dcrd/dcrjson"
)

const (
	showHelpMessage = "Specify -h to show available options"
	listCmdMessage  = "Specify -l to list available commands"
)

// commandUsage display the usage for a specific command.
func commandUsage(method string) {
	usage, err := dcrjson.MethodUsageText(method)
	if err != nil {
		// This should never happen since the method was already checked
		// before calling this function, but be safe.
		fmt.Fprintln(os.Stderr, "Failed to obtain command usage:", err)
		return
	}

	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %s\n", usage)
}

// usage displays the general usage when the help flag is not displayed and
// and an invalid command was specified.  The commandUsage function is used
// instead when a valid command was specified.
func usage(errorMessage string) {
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	fmt.Fprintln(os.Stderr, errorMessage)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %s [OPTIONS] <command> <args...>\n\n",
		appName)
	fmt.Fprintln(os.Stderr, showHelpMessage)
	fmt.Fprintln(os.Stderr, listCmdMessage)
}

func main() {
	cfg, args, err := loadConfig()
	if err != nil {
		os.Exit(1)
	}

	if len(args) < 1 {
		usage("No command specified")
		os.Exit(1)
	}

	// Ensure the specified method identifies a valid registered command and
	// is one of the usable types.
	method := args[0]
	usageFlags, err := dcrjson.MethodUsageFlags(method)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unrecognized command '%s'\n", method)
		fmt.Fprintln(os.Stderr, listCmdMessage)
		os.Exit(1)
	}
	if usageFlags&unusableFlags != 0 {
		fmt.Fprintf(os.Stderr, "The '%s' command can only be used via "+
			"websockets\n", method)
		fmt.Fprintln(os.Stderr, listCmdMessage)
		os.Exit(1)
	}

	// Convert remaining command line args to a slice of interface values
	// to be passed along as parameters to new command creation function.
	//
	// Since some commands, such as submitblock, can involve data which is
	// too large for the Operating System to allow as a normal command line
	// parameter, support using '-' as an argument to allow the argument
	// to be read from a stdin pipe.
	bio := bufio.NewReader(os.Stdin)
	params := make([]interface{}, 0, len(args[1:]))
	for _, arg := range args[1:] {
		if arg == "-" {
			param, err := bio.ReadString('\n')
			if err != nil && err != io.EOF {
				fmt.Fprintf(os.Stderr, "Failed to read data "+
					"from stdin: %v\n", err)
				os.Exit(1)
			}
			if err == io.EOF && len(param) == 0 {
				fmt.Fprintln(os.Stderr, "Not enough lines "+
					"provided on stdin")
				os.Exit(1)
			}
			param = strings.TrimRight(param, "\r\n")
			params = append(params, param)
			continue
		}

		params = append(params, arg)
	}

	// Attempt to create the appropriate command using the arguments
	// provided by the user.
	cmd, err := dcrjson.NewCmd(method, params...)
	if err != nil {
		// Show the error along with its error code when it's a
		// dcrjson.Error as it reallistcally will always be since the
		// NewCmd function is only supposed to return errors of that
		// type.
		if jerr, ok := err.(dcrjson.Error); ok {
			fmt.Fprintf(os.Stderr, "%s command: %v (code: %s)\n",
				method, err, jerr.Code)
			commandUsage(method)
			os.Exit(1)
		}

		// The error is not a dcrjson.Error and this really should not
		// happen.  Nevertheless, fallback to just showing the error
		// if it should happen due to a bug in the package.
		fmt.Fprintf(os.Stderr, "%s command: %v\n", method, err)
		commandUsage(method)
		os.Exit(1)
	}

	// Marshal the command into a JSON-RPC byte slice in preparation for
	// sending it to the RPC server.
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", 1, cmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Send the JSON-RPC request to the server using the user-specified
	// connection configuration.
	result, err := sendPostRequest(marshalledJSON, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Choose how to display the result based on its type.
	strResult := string(result)
	if strings.HasPrefix(strResult, "{") || strings.HasPrefix(strResult, "[") {
		var dst bytes.Buffer
		if err := json.Indent(&dst, result, "", "  "); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to format result: %v",
				err)
			os.Exit(1)
		}
		fmt.Println(dst.String())

	} else if strings.HasPrefix(strResult, `"`) {
		var str string
		if err := json.Unmarshal(result, &str); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to unmarshal result: %v",
				err)
			os.Exit(1)
		}
		fmt.Println(str)

	} else if strResult != "null" {
		fmt.Println(strResult)
	}
}

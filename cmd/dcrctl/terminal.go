// terminal
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/decred/dcrd/dcrjson"
	"golang.org/x/crypto/ssh/terminal"
)

func execute(protected *bool, cfg *config, line string, clear *bool) bool {
	switch line {
	case "h":
		fmt.Printf("[h]elp          print this message\n")
		fmt.Printf("[l]ist          list all available commands\n")
		fmt.Printf("[p]rotect       toggle protected mode (for passwords)\n")
		fmt.Printf("[c]lear         clear command history\n")
		fmt.Printf("[q]uit/ctrl+d   exit\n")
		fmt.Printf("Enter commands with arguments to execute them.\n")
	case "l":
		fallthrough
	case "list":
		listCommands()
	case "q":
		fallthrough
	case "quit":
		return true
	case "p":
		fallthrough
	case "protect":
		if *protected {
			*protected = false
			return false
		}
		*protected = true
		return false
	case "c":
		fallthrough
	case "clear":
		*clear = true
	default:
		args := strings.Split(line, " ")

		if len(args) < 1 {
			usage("No command specified")
			return false
		}

		// Ensure the specified method identifies a valid registered command and
		// is one of the usable types.
		listCmdMessageLocal := "Enter [l]ist to list commands"
		method := args[0]
		usageFlags, err := dcrjson.MethodUsageFlags(method)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unrecognized command '%s'\n", method)
			fmt.Fprintln(os.Stderr, listCmdMessageLocal)
			return false
		}
		if usageFlags&unusableFlags != 0 {
			fmt.Fprintf(os.Stderr, "The '%s' command can only be used via "+
				"websockets\n", method)
			fmt.Fprintln(os.Stderr, listCmdMessageLocal)
			return false
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
					return false
				}
				if err == io.EOF && len(param) == 0 {
					fmt.Fprintln(os.Stderr, "Not enough lines "+
						"provided on stdin")
					return false
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
				return false
			}

			// The error is not a dcrjson.Error and this really should not
			// happen.  Nevertheless, fallback to just showing the error
			// if it should happen due to a bug in the package.
			fmt.Fprintf(os.Stderr, "%s command: %v\n", method, err)
			commandUsage(method)
			return false
		}

		// Marshal the command into a JSON-RPC byte slice in preparation for
		// sending it to the RPC server.
		marshalledJSON, err := dcrjson.MarshalCmd(1, cmd)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return false
		}

		// Send the JSON-RPC request to the server using the user-specified
		// connection configuration.
		result, err := sendPostRequest(marshalledJSON, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return false
		}

		// Choose how to display the result based on its type.
		strResult := string(result)
		if strings.HasPrefix(strResult, "{") || strings.HasPrefix(strResult, "[") {
			var dst bytes.Buffer
			if err := json.Indent(&dst, result, "", "  "); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to format result: %v\n",
					err)
				return false
			}
			fmt.Println(dst.String())
			return false

		} else if strings.HasPrefix(strResult, `"`) {
			var str string
			if err := json.Unmarshal(result, &str); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to unmarshal result: %v\n",
					err)
				return false
			}
			fmt.Println(str)
			return false

		} else if strResult != "null" {
			fmt.Println(strResult)
		}
	}
	return false
}

func startTerminal(c *config) {
	var clear, protected bool

	fmt.Println("Starting terminal mode.")
	fmt.Println("Enter h for [h]elp.")
	fmt.Println("Enter l for [l]ist of commands.")
	fmt.Println("Enter q for [q]uit.")

	termState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set raw mode on STDIN: %v\n",
			err)
		return
	}
	n := terminal.NewTerminal(os.Stdin, "> ")
	for {
		var ln string
		var err error
		if !protected {
			ln, err = n.ReadLine()
		} else {
			ln, err = n.ReadPassword(">*")
		}
		terminal.Restore(int(os.Stdin.Fd()), termState)
		if err != nil {
			break
		}

		quit := execute(&protected, c, ln, &clear)
		if quit {
			break
		}

		if clear {
			fmt.Println("Clearing history...")
			termState, err = terminal.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to set raw "+
					"mode on STDIN: %v\n", err)
				break
			}
			n = terminal.NewTerminal(os.Stdin, "> ")
			clear = false
		} else {
			termState, err = terminal.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to set raw "+
					"mode on STDIN: %v\n", err)
				break
			}
		}
	}
	fmt.Println("exiting...")
}

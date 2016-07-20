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

func execute(quit chan bool, protected *bool, cfg *config, line string, clear *bool) {
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
		quit <- true
	case "p":
		fallthrough
	case "protect":
		if *protected {
			*protected = false
			return
		}
		*protected = true
		return
	case "c":
		fallthrough
	case "clear":
		*clear = true
	default:
		args := strings.Split(line, " ")

		if len(args) < 1 {
			usage("No command specified")
			return
		}

		// Ensure the specified method identifies a valid registered command and
		// is one of the usable types.
		listCmdMessageLocal := "Enter [l]ist to list commands"
		method := args[0]
		usageFlags, err := dcrjson.MethodUsageFlags(method)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unrecognized command '%s'\n", method)
			fmt.Fprintln(os.Stderr, listCmdMessageLocal)
			return
		}
		if usageFlags&unusableFlags != 0 {
			fmt.Fprintf(os.Stderr, "The '%s' command can only be used via "+
				"websockets\n", method)
			fmt.Fprintln(os.Stderr, listCmdMessageLocal)
			return
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
					return
				}
				if err == io.EOF && len(param) == 0 {
					fmt.Fprintln(os.Stderr, "Not enough lines "+
						"provided on stdin")
					return
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
				return
			}

			// The error is not a dcrjson.Error and this really should not
			// happen.  Nevertheless, fallback to just showing the error
			// if it should happen due to a bug in the package.
			fmt.Fprintf(os.Stderr, "%s command: %v\n", method, err)
			commandUsage(method)
			return
		}

		// Marshal the command into a JSON-RPC byte slice in preparation for
		// sending it to the RPC server.
		marshalledJSON, err := dcrjson.MarshalCmd(1, cmd)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}

		// Send the JSON-RPC request to the server using the user-specified
		// connection configuration.
		result, err := sendPostRequest(marshalledJSON, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}

		// Choose how to display the result based on its type.
		strResult := string(result)
		if strings.HasPrefix(strResult, "{") || strings.HasPrefix(strResult, "[") {
			var dst bytes.Buffer
			if err := json.Indent(&dst, result, "", "  "); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to format result: %v",
					err)
				return
			}
			fmt.Println(dst.String())
			return

		} else if strings.HasPrefix(strResult, `"`) {
			var str string
			if err := json.Unmarshal(result, &str); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to unmarshal result: %v",
					err)
				return
			}
			fmt.Println(str)
			return

		} else if strResult != "null" {
			fmt.Println(strResult)
		}
	}
}

func startTerminal(c *config) {
	fmt.Printf("Starting terminal mode.\n")
	fmt.Printf("Enter h for [h]elp.\n")
	fmt.Printf("Enter q for [q]uit.\n")
	done := make(chan bool)
	initState, err := terminal.GetState(0)
	protected := false
	clear := false
	if err != nil {
		fmt.Printf("error getting terminal state: %v\n", err.Error())
		return
	}

	go func() {
		terminal.MakeRaw(int(os.Stdin.Fd()))
		n := terminal.NewTerminal(os.Stdin, "> ")
		for {
			var ln string
			var err error
			if !protected {
				ln, err = n.ReadLine()
				if err != nil {
					done <- true
				}
			} else {
				ln, err = n.ReadPassword(">*")
				if err != nil {
					done <- true
				}
			}
			execute(done, &protected, c, ln, &clear)
			if clear {
				fmt.Println("Clearing history...")
				terminal.MakeRaw(int(os.Stdin.Fd()))
				n = terminal.NewTerminal(os.Stdin, "> ")
				clear = false
			}
		}
	}()
	select {
	case <-done:
		fmt.Printf("exiting...\n")
		terminal.Restore(0, initState)
		close(done)
	}
}

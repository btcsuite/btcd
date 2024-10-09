package btcdctrl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/internal/config"
	"github.com/btcsuite/btcd/rpcclient"
)

type Config = config.Config

func NewDefaultConfig() *Config {
	cfg := config.NewDefaultConfig()

	// Remove the configuration file, as everything is supplied in flags.
	cfg.ConfigFile = ""

	return &cfg
}

// Create a new random address.
func newRandAddress(chain *chaincfg.Params) (btcutil.Address, error) {
	// Generate a new private key.
	prv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	// Derive a public key, then serialize it.
	pk := prv.PubKey().SerializeUncompressed()

	// Create a new pay-to-pubkey address.
	return btcutil.NewAddressPubKey(pk, chain)
}

func NewTestConfig(tmp string) (*Config, error) {
	cfg := NewDefaultConfig()

	// Set the network to simnet.
	cfg.SimNet = true

	// Enable generating blocks.
	cfg.Generate = true

	// Create a new random address.
	addr, err := newRandAddress(&chaincfg.SimNetParams)
	if err != nil {
		return nil, err
	}

	// Use the random address as the mining address, required for generate.
	cfg.MiningAddrs = []btcutil.Address{addr}

	// Set the default credentials to user:pass.
	cfg.RPCUser = "user"
	cfg.RPCPass = "pass"

	// Change the root directory to the temporary directory.
	cfg.ChangeRoot(tmp)

	// Listen on 127.0.0.1 on an OS assigned port.
	cfg.RPCListeners = []string{"127.0.0.1:0"}
	cfg.Listeners = []string{"127.0.0.1:0"}

	return cfg, nil
}

type ControllerConfig struct {
	Stderr io.Writer
	Stdout io.Writer

	*Config
}

type Controller struct {
	*rpcclient.Client

	cmd *exec.Cmd

	rpc string
	p2p string

	cfg *ControllerConfig
}

// gracefully shutdown btcd instance using SIGINT and wait for it to exit.
func (c *Controller) Stop() error {
	// Signal SIGINT, for graceful shutdown.
	err := c.cmd.Process.Signal(os.Interrupt)
	if err != nil {
		return err
	}

	// Wait for the program to exit.
	for {
		exit, err := c.cmd.Process.Wait()
		if err != nil {
			break
		}

		// Check if the program exited.
		if exit.Exited() {
			break
		}
	}

	return nil
}

func (c *Controller) RPCAddress() string {
	return c.rpc
}

func (c *Controller) P2PAddress() string {
	return c.p2p
}

func (c *Controller) RPCConnConfig() (*rpcclient.ConnConfig, error) {
	cert, err := os.ReadFile(c.cfg.Config.RPCCert)
	if err != nil {
		return nil, err
	}

	return &rpcclient.ConnConfig{
		Host: c.rpc,

		User: c.cfg.RPCUser,
		Pass: c.cfg.RPCPass,

		Certificates: cert,

		HTTPPostMode: true,
	}, nil
}

// Start btcd and wait for the RPC to be ready.
func (c *Controller) Start() error {
	var args []string

	// First, we convert the `Config` type into flags.

	// struct value.
	sv := reflect.ValueOf(*c.cfg.Config)

	// Iterate over visible fields.
	for i, ft := range reflect.VisibleFields(sv.Type()) {
		// Skip unexported fields, unused by the flag parser.
		if !ft.IsExported() {
			continue
		}

		// Check for a long tag, we'll use this over the short tag to ease debugging.
		name := ft.Tag.Get("long")

		// Skip any fields without a tag, likely unused by the flag parser.
		if name == "" {
			continue
		}

		// Get the field value.
		fv := sv.Field(i)

		var val string

		// Encode the field value to a string.
		switch iface := fv.Interface(); field := iface.(type) {
		case time.Duration:
			// Encode the duration into milliseconds.
			val = fmt.Sprintf("%dms", field.Milliseconds())

		case string:
			// Skip empty fields, indicating they're unused.
			if field == "" {
				continue
			}

			// Quote the string to avoid command injection.
			val = strconv.Quote(field)

		case []string:
			// Skip empty slices, indicates unused.
			if len(field) == 0 {
				continue
			}

			// Handle listeners differently, each of them need their own flag.
			if name == "listen" || name == "rpclisten" {
				for _, f := range field {
					args = append(args, fmt.Sprintf("--%s", name), strconv.Quote(f))
				}

				continue
			}

			// Quote the string to avoid command injection.
			val = strconv.Quote(strings.Join(field, ","))

		case bool:
			// Lack of the flag means false, so skip.
			if !field {
				continue
			}

		// Encode all numbers via `fmt`, handles edge cases for us.
		case float64:
			val = fmt.Sprintf("%f", fv.Float())

		case uint, uint16, uint32, uint64:
			val = fmt.Sprintf("%d", fv.Uint())

		case int, int16, int32, int64, btcutil.Amount:
			val = fmt.Sprintf("%d", fv.Int())

		// Technically a valid value for slices, should be skipped.
		case nil:
			continue

		default:
			return errors.New("unknown type")
		}

		// Append the flag name.
		args = append(args, fmt.Sprintf("--%s", name))

		// Append the flag value if found (skipped on bools).
		if val != "" {
			args = append(args, val)
		}
	}

	// Handle fields that depend on custom encoding/decoding.

	// Encode checkpoints.
	for _, chk := range c.cfg.Config.AddCheckpoints {
		args = append(args, "--addcheckpoint", fmt.Sprintf("%d:%s", chk.Height, chk.Hash))
	}

	// Encode mining addresses.
	for _, addr := range c.cfg.Config.MiningAddrs {
		args = append(args, "--miningaddr", addr.EncodeAddress())
	}

	// Encode whitelists.
	for _, addr := range c.cfg.Config.Whitelists {
		args = append(args, "--whitelist", addr.String())
	}

	// Create the command.
	c.cmd = exec.Command("btcd", args...)

	// Create a pipe of stdout.
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}

	// Match the output configuration.
	c.cmd.Stderr = c.cfg.Stderr
	c.cmd.Stdout = pw

	// Execute the command.
	err = c.cmd.Start()
	if err != nil {
		return err
	}

	var r io.Reader = pr

	// Pipe the early output to stdout if configured.
	if c.cfg.Stdout != nil {
		r = io.TeeReader(pr, c.cfg.Stdout)
	}

	// Scan the stdout line by line.
	scan := bufio.NewScanner(r)

	rpc := c.cfg.DisableRPC
	p2p := false

	// Scan each line until both RPC (if enabled) and P2P addresses are found.
	for !rpc || !p2p {
		line := scan.Text()

		_, addr, ok := strings.Cut(line, "RPC server listening on ")
		if ok {
			c.rpc = addr
			rpc = true
		}

		_, addr, ok = strings.Cut(line, "Server listening on ")
		if !ok {
			c.p2p = addr
			p2p = true
		}

		// Ensure we've not found the RPC and P2P addresses, so we can continue scanning.
		if (!rpc || !p2p) && !scan.Scan() {
			break
		}
	}

	// Return early if RPC is disabled.
	if !rpc {
		return nil
	}

	// Discard as a fallback.
	stdout := io.Discard

	// Use the configured stdout by default.
	if c.cfg.Stdout != nil {
		stdout = c.cfg.Stdout
	}

	// The pipe needs to continuously be read, otherwise `btcd` will hang.
	go io.Copy(stdout, pr)

	deadline := time.Now().Add(30 * time.Second)

	// Try to connect via RPC for 30 seconds.
	for deadline.After(time.Now()) {
		var cfg *rpcclient.ConnConfig
		// Create the RPC config.
		cfg, err = c.RPCConnConfig()
		if err != nil {
			continue
		}

		// Create the RPC client.
		c.Client, err = rpcclient.New(cfg, nil)
		if err != nil {
			continue
		}

		// Ping the RPC client.
		err = c.Client.Ping()
		if err != nil {
			continue
		}

		err = nil
		break
	}

	// Check if the connection loop exited with an error.
	if err != nil {
		return errors.Join(errors.New("timeout"), err)
	}

	// Check if the client was created.
	if c.Client == nil {
		return errors.New("timeout")
	}

	return nil
}

func New(cfg *ControllerConfig) *Controller {
	return &Controller{
		cfg: cfg,
	}
}

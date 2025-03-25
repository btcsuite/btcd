package btcdtest

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/internal/config"
	"github.com/btcsuite/btcd/rpcclient"
)

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

// Create the default configuration for testing. Ranomized ports for RPC & P2P, RPC user is "user", pass is "pass".
// Random mining address.
func defaultConfig(tmp string) (*Config, error) {
	cfg := config.NewDefaultConfig()

	// Remove the configuration file, as everything is supplied in flags.
	cfg.ConfigFile = ""

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

	return &Config{
		Config: &cfg,
	}, nil
}

type Config struct {
	Stderr io.Writer
	Stdout io.Writer

	Path string

	*config.Config
}

type Harness struct {
	*rpcclient.Client

	cmd *exec.Cmd

	rpc string
	p2p string

	cfg *Config

	running atomic.Bool
	stopped atomic.Bool
}

// Gracefully shutdown btcd instance using SIGINT and wait for it to exit.
func (h *Harness) Stop() {
	if !h.stopped.CompareAndSwap(false, true) {
		h.running.Store(false)
		return
	}

	// Signal SIGINT, for graceful shutdown.
	err := h.cmd.Process.Signal(os.Interrupt)
	if err != nil {
		panic(err)
	}

	// Wait for the program to exit.
	for {
		exit, err := h.cmd.Process.Wait()
		if err != nil {
			break
		}

		// Check if the program exited.
		if exit.Exited() {
			break
		}
	}
}

func (c *Harness) RPCAddress() string {
	return c.rpc
}

func (c *Harness) P2PAddress() string {
	return c.p2p
}

func (c *Harness) RPCConnConfig() (*rpcclient.ConnConfig, error) {
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
func (h *Harness) Start() {
	if !h.running.CompareAndSwap(false, true) {
		return
	}

	var args []string

	// First, we convert the `Config` type into flags.

	// struct value.
	sv := reflect.ValueOf(*h.cfg.Config)

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
			panic("unknown type")
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
	for _, chk := range h.cfg.Config.AddCheckpoints {
		args = append(args, "--addcheckpoint", fmt.Sprintf("%d:%s", chk.Height, chk.Hash))
	}

	// Encode mining addresses.
	for _, addr := range h.cfg.Config.MiningAddrs {
		args = append(args, "--miningaddr", addr.EncodeAddress())
	}

	// Encode whitelists.
	for _, addr := range h.cfg.Config.Whitelists {
		args = append(args, "--whitelist", addr.String())
	}

	name := "btcd"
	if h.cfg.Path != "" {
		name = h.cfg.Path
	}

	// Create the command.
	h.cmd = exec.Command(name, args...)

	// Create a pipe of stdout.
	pr, pw, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	// Match the output configuration.
	h.cmd.Stderr = h.cfg.Stderr
	h.cmd.Stdout = pw

	// Execute the command.
	err = h.cmd.Start()
	if err != nil {
		panic(err)
	}

	var r io.Reader = pr

	// Pipe the early output to stdout if configured.
	if h.cfg.Stdout != nil {
		r = io.TeeReader(pr, h.cfg.Stdout)
	}

	// Scan the stdout line by line.
	scan := bufio.NewScanner(r)

	rpc := h.cfg.DisableRPC
	p2p := false

	// Scan each line until both RPC (if enabled) and P2P addresses are found.
	for !rpc || !p2p {
		line := scan.Text()

		_, addr, ok := strings.Cut(line, "RPC server listening on ")
		if ok {
			h.rpc = addr
			rpc = true
		}

		_, addr, ok = strings.Cut(line, "Server listening on ")
		if ok {
			h.p2p = addr
			p2p = true
		}

		// Ensure we've not found the RPC and P2P addresses, so we can continue scanning.
		if (!rpc || !p2p) && !scan.Scan() {
			break
		}
	}

	// Return early if RPC is disabled.
	if !rpc {
		return
	}

	// Discard as a fallback.
	stdout := io.Discard

	// Use the configured stdout by default.
	if h.cfg.Stdout != nil {
		stdout = h.cfg.Stdout
	}

	// The pipe needs to continuously be read, otherwise `btcd` will hang.
	go io.Copy(stdout, pr)

	deadline := time.Now().Add(30 * time.Second)

	// Try to connect via RPC for 30 seconds.
	for deadline.After(time.Now()) {
		var cfg *rpcclient.ConnConfig
		// Create the RPC config.
		cfg, err = h.RPCConnConfig()
		if err != nil {
			continue
		}

		// Create the RPC client.
		h.Client, err = rpcclient.New(cfg, nil)
		if err != nil {
			continue
		}

		// Ping the RPC client.
		err = h.Client.Ping()
		if err != nil {
			continue
		}

		err = nil
		break
	}

	// Check if the connection loop exited with an error.
	if err != nil {
		panic(fmt.Sprintf("timeout: %v", err))
	}

	// Check if the client was created.
	if h.Client == nil {
		panic("timeout")
	}

	// Enable block generation.
	if h.cfg.Config.Generate {
		err := h.SetGenerate(true, 0)
		if err != nil {
			panic(err)
		}
	}

}

// Override the default configuration.
func WithConfig(cfg *Config) func(*Config) {
	return func(p *Config) {
		*p = *cfg
	}
}

// Update the root directory.
func WithRootDir(dir string) func(*Config) {
	return func(cfg *Config) {
		cfg.Config.ChangeRoot(dir)
	}
}

// Update the output.
func WithOutput(stderr io.Writer, stdout io.Writer) func(*Config) {
	return func(cfg *Config) {
		cfg.Stderr = stderr
		cfg.Stdout = stdout
	}
}

// Set custom debug log level.
func WithDebugLevel(level string) func(*Config) {
	return func(cfg *Config) {
		cfg.Config.DebugLevel = level
	}
}

// Set custom binary path.
func WithBinary(path string) func(*Config) {
	return func(cfg *Config) {
		cfg.Path = path
	}
}

func WithChainParams(chain *chaincfg.Params) func(*Config) {
	return func(cfg *Config) {
		cfg.Config.SimNet = false

		switch chain.Name {
		case chaincfg.TestNet3Params.Name:
			cfg.Config.TestNet3 = true

		case chaincfg.TestNet4Params.Name:
			cfg.Config.TestNet4 = true

		case chaincfg.SimNetParams.Name:
			cfg.Config.SimNet = true

		case chaincfg.MainNetParams.Name:
			// nop

		case chaincfg.SigNetParams.Name:
			cfg.Config.SigNet = true

		case chaincfg.RegressionNetParams.Name:
			cfg.Config.RegressionTest = true

		default:
			panic("unknown network")
		}

		addr, err := newRandAddress(chain)
		if err != nil {
			panic(err)
		}

		cfg.MiningAddrs = []btcutil.Address{addr}

	}
}

// Create and start a harness.
func New(opts ...func(*Config)) *Harness {
	h := NewUnstarted(opts...)
	h.Start()

	return h
}

func NewUnstarted(opts ...func(*Config)) *Harness {
	tmp, err := os.MkdirTemp("", "btcdtest-*")
	if err != nil {
		panic(err)
	}

	cfg, err := defaultConfig(tmp)
	if err != nil {
		panic(err)
	}

	for _, opt := range opts {
		opt(cfg)
	}

	h := &Harness{
		cfg: cfg,
	}

	return h
}

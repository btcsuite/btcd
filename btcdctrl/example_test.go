package btcdctrl_test

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcdctrl"
)

func ExampleController() {
	// Create a temporary directory for the wallet data.
	tmp, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}

	// Create a new test-oriented configration.
	cfg, err := btcdctrl.NewTestConfig(tmp)
	if err != nil {
		panic(err)
	}

	// Create a new controller.
	c := btcdctrl.New(&btcdctrl.ControllerConfig{
		Config: cfg,
	})

	// Start btcd.
	err = c.Start()
	if err != nil {
		panic(err)
	}

	// Stop btcd on exit.
	defer c.Stop()

	// Enable generation.
	err = c.SetGenerate(true, 0)
	if err != nil {
		panic(err)
	}

	// Generate 100 blocks.
	_, err = c.Generate(100)
	if err != nil {
		panic(err)
	}

	// Query info.
	info, err := c.GetInfo()
	if err != nil {
		panic(err)
	}

	fmt.Println(info.Blocks)
	// Output: 100
}

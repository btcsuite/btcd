package btcdtest_test

import (
	"fmt"

	"github.com/btcsuite/btcd/btcdtest"
)

func ExampleHarness() {
	// Create a new harness.
	c := btcdtest.New()

	// Stop btcd on exit.
	defer c.Stop()

	// Generate 100 blocks.
	_, err := c.Generate(100)
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

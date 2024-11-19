package btcdctrl

import (
	"os"
	"testing"
)

func TestFail(t *testing.T) {
	for i := 0; i < 100; i++ {
		cfg, err := NewTestConfig(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		cfg.DebugLevel = "trace"

		// Create a new controller.
		c := New(&ControllerConfig{
			Stderr: os.Stderr,
			Stdout: os.Stdout,

			Config: cfg,
		})

		// Start btcd.
		err = c.Start()
		if err != nil {
			t.Fatal(err)
		}

		// Stop btcd.
		err = c.Stop()
		if err != nil {
			t.Fatal(err)
		}
	}
}

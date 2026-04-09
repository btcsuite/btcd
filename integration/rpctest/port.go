package rpctest

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

var (
	portRangeStart = 30000
	portRangeEnd   = 40000
	portLockDir    = filepath.Join(os.TempDir(), "test_port_locks")
)

func ReservePort() (int, error) {
	// Always attempt to create the lock directory
	if err := os.MkdirAll(portLockDir, 0755); err != nil {
		return 0, err
	}

	for port := portRangeStart; port <= portRangeEnd; port++ {
		lockFile := filepath.Join(portLockDir, fmt.Sprintf("port_%d.lock", port))

		// Attempt to atomically create the lock file
		f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			// Successfully reserved the port
			defer f.Close()

			// Verify the port is actually usable
			l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				// Port is reserved but unusable, release the lock
				os.Remove(lockFile)
				continue
			}
			l.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d–%d", portRangeStart, portRangeEnd)
}

func ReleasePort(port int) error {
	lockFile := filepath.Join(portLockDir, fmt.Sprintf("port_%d.lock", port))
	return os.Remove(lockFile)
}

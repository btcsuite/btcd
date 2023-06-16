package ossec

import (
	"golang.org/x/sys/unix"
)

func Unveil(path string, perms string) error {
	return unix.Unveil(path, perms)
}

func Pledge(promises, execpromises string) error {
	return unix.Pledge(promises, execpromises)
}

func PledgePromises(promises string) error {
	return unix.PledgePromises(promises)
}

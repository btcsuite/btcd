//go:build !openbsd

package ossec

func Unveil(path string, perms string) error {
	return nil
}

func Pledge(promises, execpromises string) error {
	return nil
}

func PledgePromises(promises string) error {
	return nil
}

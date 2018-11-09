// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sleep pauses the current goroutine for at least the target milliseconds.
func Sleep(milliseconds int64) {
	//fmt.Println("sleep: " + strconv.FormatInt(milliseconds, 10))
	time.Sleep(time.Duration(milliseconds * int64(time.Millisecond)))
}

// FileExists returns true when file exists, and false otherwise
func FileExists(path string) bool {
	e, err := fileExists(path)
	if err != nil {
		return false
	}
	return e
}

func fileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WaitForFile sleeps until target file is created
// or timeout is reached
func WaitForFile(file string, maxSecondsToWait int) {
	counter := maxSecondsToWait
	for !FileExists(file) {
		fmt.Println("waiting for: " + file)
		Sleep(1000)
		counter--
		if counter < 0 {
			err := fmt.Errorf("file not found: %v", file)
			ReportTestSetupMalfunction(err)
		}
	}
}

// MakeDirs ensures target folder and all it's parents exist.
// As opposed to the os.Mkdir will not fail due to a lack of unix permissions.
func MakeDirs(dir string) {
	sep := string(os.PathSeparator)
	steps := strings.Split(dir, sep)
	for i := 1; i <= len(steps); i++ {
		pathI := filepath.Join(steps[:i]...)
		if pathI == "" {
			continue
		}
		if !FileExists(pathI) {
			err := os.Mkdir(pathI, 0755)
			CheckTestSetupMalfunction(err)
		}
	}
}

// DeleteFile ensures file was deleted
// reports test setup malfunction otherwise
func DeleteFile(file string) {
	fmt.Println("delete: " + file)
	err := os.RemoveAll(file)
	CheckTestSetupMalfunction(err)
}

// ListContainsString returns true when the list contains target string
func ListContainsString(list []string, a string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

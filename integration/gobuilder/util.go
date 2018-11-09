// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gobuilder

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/btcsuite/btcd/integration"
)

// DetermineProjectPackagePath is used to determine target project path
// for the following execution of a Go builder.
// Starts from a current working directory, climbs up to a parent folder
// with the target name and returns its path as a result.
func DetermineProjectPackagePath(projectName string) string {
	// Determine import path of this package.
	_, launchDir, _, ok := runtime.Caller(1)
	if !ok {
		integration.CheckTestSetupMalfunction(
			fmt.Errorf("cannot get project <%v> path, launch dir is: %v ",
				projectName,
				launchDir,
			),
		)
	}
	sep := "/"
	steps := strings.Split(launchDir, sep)
	for i, s := range steps {
		if s == projectName {
			pkgPath := strings.Join(steps[:i+1], "/")
			return pkgPath
		}
	}
	integration.CheckTestSetupMalfunction(
		fmt.Errorf("cannot get project <%v> path, launch dir is: %v ",
			projectName,
			launchDir,
		),
	)
	return ""
}

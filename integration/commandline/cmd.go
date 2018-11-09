// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package commandline

import "fmt"

var (
	// NoArgumentValue indicates flag has name but no value to provide,
	// example: "--someflag"
	NoArgumentValue interface{} = &struct{}{} // stub object

	// NoArgument indicates the argument should be omitted from console command
	NoArgument = ""
	// NoArgumentNil indicates the argument should be omitted from console command
	NoArgumentNil interface{} // =nil

	// See ArgumentsToStringArray to understand how these constants are used
)

// ArgumentsToStringArray converts map to an array of command line arguments
// taking in account NoArgumentValue and NoArgument indicators above
func ArgumentsToStringArray(args map[string]interface{}) []string {
	var result []string
	for key, value := range args {
		if value == NoArgument || value == NoArgumentNil {
			// skip key
		} else if value == NoArgumentValue {
			// --%key%
			str := fmt.Sprintf("--%s", key)
			result = append(result, str)
		} else {
			// --%key%=%value%
			str := fmt.Sprintf("--%s=%s", key, value)
			result = append(result, str)
		}
	}
	return result
}

// ArgumentsCopyTo helps to append commandline arguments from one map to another
func ArgumentsCopyTo(from map[string]interface{}, to map[string]interface{}) map[string]interface{} {
	for key, value := range from {
		to[key] = value
	}
	return to
}

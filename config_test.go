package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"
)

var (
	rpcuserRegexp = regexp.MustCompile("(?m)^rpcuser=.+$")
	rpcpassRegexp = regexp.MustCompile("(?m)^rpcpass=.+$")
)

// Define a struct "configCmdLineOnly" containing a subset of configuration
// parameters which are command-line only. These fields are copied line-by-line
// from "config" struct in "config.go", and the field names, types, and tags must
// match for the test to work.
//
type configCmdLineOnly struct {
	ConfigFile          string   `short:"C" long:"configfile" description:"Path to configuration file"`
	DbType              string   `long:"dbtype" description:"Database backend to use for the Block Chain"`
	DropCfIndex         bool     `long:"dropcfindex" description:"Deletes the index used for committed filtering (CF) support from the database on start up and then exits."`
	DropTxIndex         bool     `long:"droptxindex" description:"Deletes the hash-based transaction index from the database on start up and then exits."`
	DisableCheckpoints  bool     `long:"nocheckpoints" description:"Disable built-in checkpoints.  Don't do this unless you know what you're doing."`
	NoWinService        bool     `long:"nowinservice" description:"Do not start as a background service on Windows -- NOTE: This flag only works on the command line, not in the config file"`
	DisableStallHandler bool     `long:"nostalldetect" description:"Disables the stall handler system for each peer, useful in simnet/regtest integration tests frameworks"`
	RegressionTest      bool     `long:"regtest" description:"Use the regression test network"`
	SimNet              bool     `long:"simnet" description:"Use the simulation test network"`
	SigNet              bool     `long:"signet" description:"Use the signet test network"`
	SigNetChallenge     string   `long:"signetchallenge" description:"Connect to a custom signet network defined by this challenge instead of using the global default signet test network -- Can be specified multiple times"`
	SigNetSeedNode      []string `long:"signetseednode" description:"Specify a seed node for the signet network instead of using the global default signet network seed nodes"`
	ShowVersion         bool     `short:"V" long:"version" description:"Display version information and exit"`
}

func fieldEq(f1, f2 reflect.StructField) bool {
	return (f1.Name == f2.Name) && (f1.Type == f2.Type) && (f1.Tag == f2.Tag)
}

func TestSampleConfigFileComplete(t *testing.T) {
	// find out where the sample config lives
	_, path, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed finding config file path")
	}
	sampleConfigFile := filepath.Join(filepath.Dir(path), sampleConfigFilename)

	// Read the sample config file
	content, err := ioutil.ReadFile(sampleConfigFile)
	if err != nil {
		t.Fatalf("Failed reading sample config file: %v", err)
	}

	allFields := reflect.VisibleFields(reflect.TypeOf(config{}))
	cmdlineFields := reflect.VisibleFields(reflect.TypeOf(configCmdLineOnly{}))

	// Verify cmdlineFields is a subset of allFields.
	for _, cf := range cmdlineFields {
		// Check for presence of field "cf" in config struct.
		var field *reflect.StructField
		for _, f := range allFields {
			f := f // new instance of loop var for return
			if fieldEq(cf, f) {
				field = &f
				break
			}
		}
		if field == nil {
			t.Errorf("cmdline field: %s type: %s is not present in type %s",
				cf.Name, cf.Type, reflect.TypeOf(config{}))
		}
	}

	// Verify sample config covers all parameters.
	for _, f := range allFields {
		longname, ok := f.Tag.Lookup("long")
		if !ok {
			// Field has no long-form name, so not eligible for
			// inclusion in sample config.
			continue
		}

		// Check for presence of field "f" in our configCmdLineOnly struct.
		var cmdline *reflect.StructField
		for _, cf := range cmdlineFields {
			cf := cf // new instance of loop var for return
			if fieldEq(cf, f) {
				cmdline = &cf
				break
			}
		}

		// Look for assignment (<longname>="), or commented assignment ("; <longname>=").
		pattern := fmt.Sprintf("(?m)^(;\\s*)?%s=.*$", longname)
		assignment, err := regexp.Compile(pattern)
		if err != nil {
			t.Errorf("config field: %s longname: %s failed compiling regexp (%s): %v",
				f.Name, longname, pattern, err)
			continue
		}

		assigned := assignment.Match(content)

		// Field "f" must be present in either the sample config (<longname>=X),
		// or it must be one of the command line only fields, but not both.
		if !assigned && (cmdline == nil) {
			t.Errorf("config field: %s longname: %s assignment (%s) should be present in %s",
				f.Name, longname, assignment, sampleConfigFilename)
		}
		if assigned && (cmdline != nil) {
			t.Errorf("config field: %s longname: %s should not be present in both %s and type %s",
				f.Name, longname, sampleConfigFilename, reflect.TypeOf(configCmdLineOnly{}).Name())
		}
	}
}

func TestCreateDefaultConfigFile(t *testing.T) {
	// find out where the sample config lives
	_, path, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed finding config file path")
	}
	sampleConfigFile := filepath.Join(filepath.Dir(path), "sample-lbcd.conf")

	// Setup a temporary directory
	tmpDir, err := ioutil.TempDir("", "lbcd")
	if err != nil {
		t.Fatalf("Failed creating a temporary directory: %v", err)
	}
	testpath := filepath.Join(tmpDir, "test.conf")

	// copy config file to location of lbcd binary
	data, err := ioutil.ReadFile(sampleConfigFile)
	if err != nil {
		t.Fatalf("Failed reading sample config file: %v", err)
	}
	appPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		t.Fatalf("Failed obtaining app path: %v", err)
	}
	tmpConfigFile := filepath.Join(appPath, "sample-lbcd.conf")
	err = ioutil.WriteFile(tmpConfigFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed copying sample config file: %v", err)
	}

	// Clean-up
	defer func() {
		os.Remove(testpath)
		os.Remove(tmpConfigFile)
		os.Remove(tmpDir)
	}()

	err = createDefaultConfigFile(testpath)

	if err != nil {
		t.Fatalf("Failed to create a default config file: %v", err)
	}

	content, err := ioutil.ReadFile(testpath)
	if err != nil {
		t.Fatalf("Failed to read generated default config file: %v", err)
	}

	if !rpcuserRegexp.Match(content) {
		t.Error("Could not find rpcuser in generated default config file.")
	}

	if !rpcpassRegexp.Match(content) {
		t.Error("Could not find rpcpass in generated default config file.")
	}
}

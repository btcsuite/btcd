package version

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
)

var appTag = "v0.0.0-local.0"

// Full returns full version string conforming to semantic versioning 2.0.0
// spec (http://semver.org/).
//
//   Major.Minor.Patch-Prerelease+Buildmeta
//
// Prerelease must be either empty or in the form of Phase.Revision. The Phase
// must be local, dev, alpha, beta, or rc.
// Buildmeta is full length of 40-digit git commit ID with "-dirty" appended
// refelecting uncommited chanegs.
//
// This function relies injected git version tag in the form of:
//
//   vMajor.Minor.Patch-Prerelease
//
// The injection can be done with go build flags for example:
//
//   go build -ldflags "-X github.com/lbryio/lbcd/version.appTag=v1.2.3-beta.45"
//
// Without explicitly injected tag, a default one - "v0.0.0-local.0" is used
// indicating a local development build.

// The version is encoded into a int32 numeric form, which imposes valid ranges
// on each component:
//
//   Major: 0 - 41
//   Minor: 0 - 99
//   Patch: 0 - 999
//
//   Prerelease: Phase.Revision
//     Phase: [ local | dev | alpha | beta | rc | ]
//     Revision: 0 - 99
//
//   Buildmeta: CommitID or CommitID-dirty
//
// Examples:
//
//   1.2.3-beta.45+950b68348261e0b4ff288d216269b8ad2a384411
//   2.6.4-alpha.3+92d00aaee19d1709ae64b36682ae9897ef91a2ca-dirty

func Full() string {
	return parsed.full()
}

// Numeric returns numeric form of full version (excluding meta) in a 32-bit decimal number.
// See Full() for more details.
func Numeric() int32 {
	numeric := parsed.major*100000000 +
		parsed.minor*1000000 +
		parsed.patch*1000 +
		parsed.phase.numeric()*100 +
		parsed.revision

	return int32(numeric)
}

func init() {

	version, prerelease, err := parseTag(appTag)
	if err != nil {
		panic(fmt.Errorf("parse tag: %s; %w", appTag, err))
	}

	major, minor, patch, err := parseVersion(version)
	if err != nil {
		panic(fmt.Errorf("parse version: %s; %w", version, err))
	}

	phase, revision, err := parsePrerelease(prerelease)
	if err != nil {
		panic(fmt.Errorf("parse prerelease: %s; %w", prerelease, err))
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		panic(fmt.Errorf("binary must be built with Go 1.18+ with module support"))
	}

	var commit string
	var modified bool
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			commit = s.Value
		}
		if s.Key == "vcs.modified" && s.Value == "true" {
			modified = true
		}
	}

	parsed = parsedVersion{
		version: version,
		major:   major,
		minor:   minor,
		patch:   patch,

		prerelease: prerelease,
		phase:      phase,
		revision:   revision,

		commit:   commit,
		modified: modified,
	}
}

var parsed parsedVersion

type parsedVersion struct {
	version string
	// Semantic Version
	major int
	minor int
	patch int

	// Prerelease
	prerelease string
	phase      releasePhase
	revision   int

	// Build Metadata
	commit   string
	modified bool
}

func (v parsedVersion) buildmeta() string {
	if !v.modified {
		return v.commit
	}
	return v.commit + "-dirty"
}

func (v parsedVersion) full() string {
	return fmt.Sprintf("%s-%s+%s", v.version, v.prerelease, v.buildmeta())
}

func parseTag(tag string) (version string, prerelease string, err error) {

	if len(tag) == 0 || tag[0] != 'v' {
		return "", "", fmt.Errorf("tag must be prefixed with v; %s", tag)
	}

	strs := strings.Split(tag[1:], "-")

	if len(strs) != 2 {
		return "", "", fmt.Errorf("tag must be in the form of Version.Revision; %s", tag)
	}

	version = strs[0]
	prerelease = strs[1]

	return version, prerelease, nil
}

func parseVersion(ver string) (major int, minor int, patch int, err error) {

	strs := strings.Split(ver, ".")

	if len(strs) != 3 {
		return major, minor, patch, fmt.Errorf("invalid format; must be in the form of Major.Minor.Patch")
	}

	major, err = strconv.Atoi(strs[0])
	if err != nil {
		return major, minor, patch, fmt.Errorf("invalid major: %s", strs[0])
	}
	if major < 0 || major > 41 {
		return major, minor, patch, fmt.Errorf("major must between 0 - 41; got %d", major)
	}

	minor, err = strconv.Atoi(strs[1])
	if err != nil {
		return major, minor, patch, fmt.Errorf("invalid minor: %s", strs[1])
	}
	if minor < 0 || minor > 99 {
		return major, minor, patch, fmt.Errorf("minor must between 0 - 99; got %d", minor)
	}

	patch, err = strconv.Atoi(strs[2])
	if err != nil {
		return major, minor, patch, fmt.Errorf("invalid patch: %s", strs[2])
	}
	if patch < 0 || patch > 999 {
		return major, minor, patch, fmt.Errorf("patch must between 0 - 999; got %d", patch)
	}

	return major, minor, patch, nil
}

func parsePrerelease(pre string) (phase releasePhase, revision int, err error) {

	phase = Unkown

	if pre == "" {
		return GA, 0, nil
	}

	strs := strings.Split(pre, ".")
	if len(strs) != 2 {
		return phase, revision, fmt.Errorf("prerelease must be in the form of Phase.Revision; got: %s", pre)
	}

	phase = releasePhase(strs[0])
	if phase.numeric() == -1 {
		return phase, revision, fmt.Errorf("phase must be local, dev, alpha, beta, or rc; got: %s", strs[0])
	}

	revision, err = strconv.Atoi(strs[1])
	if err != nil {
		return phase, revision, fmt.Errorf("invalid revision: %s", strs[0])
	}
	if revision < 0 || revision > 99 {
		return phase, revision, fmt.Errorf("revision must between 0 - 999; got %d", revision)
	}

	return phase, revision, nil
}

type releasePhase string

const (
	Unkown releasePhase = "unkown"
	Local  releasePhase = "local"
	Dev    releasePhase = "dev"
	Alpha  releasePhase = "alpha"
	Beta   releasePhase = "beta"
	RC     releasePhase = "rc"
	GA     releasePhase = ""
)

func (p releasePhase) numeric() int {

	switch p {
	case Local:
		return 0
	case Dev:
		return 1
	case Alpha:
		return 2
	case Beta:
		return 3
	case RC:
		return 4
	case GA:
		return 5
	}

	// Unknown phase
	return -1
}

#!/usr/bin/env bash
# upgrade-to-v2-modules.sh
#
# Scripted-diff: migrate a consumer repository of github.com/btcsuite/btcd
# to the new v2 module layout introduced in btcd PRs #1825 and #2532
# (release v0.26.0).
#
# What this script does, in order:
#   1. Rewrites all btcd import paths in tracked .go files to the new v2
#      modules (chainhash/v2, wire/v2, chaincfg/v2, address/v2,
#      txscript/v2, btcutil/v2, psbt/v2).
#   2. Rewrites references to symbols that moved from the btcutil package
#      to the new address package (btcutil.Address* -> address.Address*,
#      btcutil.Hash160 -> address.Hash160, etc.).
#   3. Over-imports: every file that imports btcutil/v2 additionally gets
#      an import of address/v2. The subsequent goimports run removes
#      whichever of the two is unused in each file.
#   4. Updates every go.mod in the repository: pins the new btcd modules
#      at the released tags and adds local replace directives for sibling
#      modules that live in the same repository (their published tags are
#      still built against the old btcd modules).
#   5. Runs goimports on all tracked .go files.
#   6. Runs go mod tidy on every module and normalizes the toolchain
#      directive.
#
# The transformation is purely mechanical. Code that, for example, uses a
# local variable named "address" alongside the rewritten address.* symbols
# will not compile afterwards; such fixups are expected to land in
# separate, manually authored commits on top of the scripted-diff commit.
# Cross-repository dependencies (btcwallet <-> neutrino <-> lnd) are NOT
# bumped here; they are follow-up commits once the respective repos have
# tagged v2-compatible releases.
#
# Requirements: bash, git, GNU sed, GNU coreutils (realpath), goimports
# in $PATH, go >= 1.25 (or GOTOOLCHAIN=auto to fetch it) and network
# access to the Go module proxy.
#
# Usage: run from the root of the consumer repository:
#   ./upgrade-to-v2-modules.sh

set -euo pipefail

# The new btcd module versions, pinned to the tags published with btcd
# v0.26.0-beta.rc1.
BTCD_VERSION="v0.26.0"
BTCEC_VERSION="v2.5.0"
CHAINHASH_VERSION="v2.0.0"
WIRE_VERSION="v2.0.0"
CHAINCFG_VERSION="v2.0.0"
ADDRESS_VERSION="v2.0.0"
TXSCRIPT_VERSION="v2.0.0"
BTCUTIL_VERSION="v2.0.0"
PSBT_VERSION="v2.0.0"

# Make sure module resolution is not influenced by a go.work file outside
# of the repository.
export GOWORK=off

# ----------------------------------------------------------------------
# Sanity checks.
# ----------------------------------------------------------------------

for tool in git go goimports realpath; do
	if ! command -v "$tool" >/dev/null; then
		echo "error: required tool '$tool' not found in \$PATH" >&2
		exit 1
	fi
done

# An outdated goimports (pre go1.18) silently fails on files that use
# generics, so make sure the installed binary can parse them.
if ! printf 'package x\n\nfunc f[T any]() {}\n' | goimports >/dev/null; then
	echo "error: goimports cannot parse generics; update it with" >&2
	echo "  go install golang.org/x/tools/cmd/goimports@latest" >&2
	exit 1
fi

# GNU sed is required for the in-place edits and \n in replacements. On
# macOS it is usually available as gsed.
SED=sed
if ! sed --version 2>/dev/null | grep -q "GNU sed"; then
	if command -v gsed >/dev/null; then
		SED=gsed
	else
		echo "error: GNU sed is required (install gnu-sed)" >&2
		exit 1
	fi
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	echo "error: must be run from within a git repository" >&2
	exit 1
fi

if [[ ! -f go.mod ]]; then
	echo "error: no go.mod found; run from the repository root" >&2
	exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
	echo "error: working tree is not clean; commit or stash first" >&2
	exit 1
fi

# All tracked Go files, excluding any vendored code.
go_files() {
	git ls-files -- '*.go' ':(exclude)vendor/**' \
		':(exclude)**/vendor/**'
}

# All tracked go.mod files, excluding any vendored code.
go_mods() {
	git ls-files -- 'go.mod' '**/go.mod' ':(exclude)vendor/**' \
		':(exclude)**/vendor/**'
}

# ----------------------------------------------------------------------
# Step 1+2: rewrite import paths and moved btcutil -> address symbols.
# ----------------------------------------------------------------------

echo "==> Rewriting import paths and moved symbols in .go files"

# Notes on the import path rules:
#  - chaincfg/chainhash must be rewritten before the bare chaincfg rule.
#  - The btcutil subpackage rules must come before the bare btcutil rule.
#  - The bare package rules are anchored on the closing quote of the
#    import path so they cannot re-match paths already rewritten by the
#    subpackage rules above them (mentions of the bare paths in doc
#    comments without quotes are intentionally left alone).
#  - btcec keeps its import path (github.com/btcsuite/btcd/btcec/v2) and
#    is only version-bumped in go.mod.
#
# Notes on the symbol rules: the following symbols moved from the btcutil
# package to the new address package: the Address interface and all
# Address* types with their New* constructors, DecodeAddress, Hash160,
# PubKeyFormat with its PKF* constants, the UnsupportedWitness*Error
# types and the ErrChecksumMismatch, ErrUnknownAddressType and
# ErrAddressCollision variables. The Address prefix alternation below
# covers the interface, all concrete types and all constructors.
go_files | xargs -r "$SED" -i -E \
	-e 's|github\.com/btcsuite/btcd/chaincfg/chainhash|github.com/btcsuite/btcd/chainhash/v2|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/psbt|github.com/btcsuite/btcd/psbt/v2|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/base58|github.com/btcsuite/btcd/address/v2/base58|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/bech32|github.com/btcsuite/btcd/address/v2/bech32|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/bloom|github.com/btcsuite/btcd/btcutil/v2/bloom|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/coinset|github.com/btcsuite/btcd/btcutil/v2/coinset|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/gcs|github.com/btcsuite/btcd/btcutil/v2/gcs|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/hdkeychain|github.com/btcsuite/btcd/btcutil/v2/hdkeychain|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil/txsort|github.com/btcsuite/btcd/btcutil/v2/txsort|g' \
	-e 's|github\.com/btcsuite/btcd/btcutil"|github.com/btcsuite/btcd/btcutil/v2"|g' \
	-e 's|github\.com/btcsuite/btcd/chaincfg"|github.com/btcsuite/btcd/chaincfg/v2"|g' \
	-e 's|github\.com/btcsuite/btcd/wire"|github.com/btcsuite/btcd/wire/v2"|g' \
	-e 's|github\.com/btcsuite/btcd/txscript"|github.com/btcsuite/btcd/txscript/v2"|g' \
	-e 's/btcutil\.(Address|NewAddress|DecodeAddress|Hash160|PubKeyFormat|PKFCompressed|PKFUncompressed|UnsupportedWitnessProgLenError|UnsupportedWitnessVerError|ErrChecksumMismatch|ErrUnknownAddressType|ErrAddressCollision)/address.\1/g'

# ----------------------------------------------------------------------
# Step 3: over-import address/v2 next to every btcutil/v2 import.
# ----------------------------------------------------------------------

echo "==> Over-importing address/v2 alongside btcutil/v2"

# Insert the address/v2 import directly above the btcutil/v2 import in
# every file that does not already import it. The goimports run below
# removes the import again in files that do not reference the address
# package (and the btcutil/v2 import in files that no longer reference
# btcutil). The first expression handles imports inside a parenthesized
# import block (with an optional alias which is preserved on the btcutil
# line), the second one handles single-line import statements.
for f in $(git grep -l '"github.com/btcsuite/btcd/btcutil/v2"' -- \
	'*.go' ':(exclude)vendor/**' ':(exclude)**/vendor/**'); do

	if grep -q '"github.com/btcsuite/btcd/address/v2"' "$f"; then
		continue
	fi

	"$SED" -i -E \
		-e 's|^([[:space:]]+)([A-Za-z0-9_]+[[:space:]]+)?"github\.com/btcsuite/btcd/btcutil/v2"|\1"github.com/btcsuite/btcd/address/v2"\n\1\2"github.com/btcsuite/btcd/btcutil/v2"|' \
		-e 's|^import[[:space:]]+"github\.com/btcsuite/btcd/btcutil/v2"|import "github.com/btcsuite/btcd/address/v2"\nimport "github.com/btcsuite/btcd/btcutil/v2"|' \
		"$f"
done

# ----------------------------------------------------------------------
# Step 4: update every go.mod in the repository.
# ----------------------------------------------------------------------

echo "==> Updating go.mod files"

# Collect the module path of every module in this repository so that we
# can add local replace directives between sibling modules below.
declare -A MODULE_DIRS
for gm in $(go_mods); do
	mod_path=$(awk '/^module /{print $2; exit}' "$gm")
	MODULE_DIRS["$mod_path"]=$(dirname "$gm")
done

# Add a local replace directive for every sibling module of this
# repository that a module requires. The siblings' published tags are
# still built against the old btcd v1 modules, so without the replace
# the migrated code would mix incompatible types. This runs again after
# the first tidy pass because tidy can surface additional intra-repo
# requirements that were previously missing from a go.mod.
add_sibling_replaces() {
	local gm dir self sibling rel
	for gm in $(go_mods); do
		dir=$(dirname "$gm")
		self=$(awk '/^module /{print $2; exit}' "$gm")

		# Iterate the siblings in sorted order so that the
		# resulting replace directives are reproducible across
		# bash versions.
		for sibling in $(printf '%s\n' "${!MODULE_DIRS[@]}" |
			sort); do
			if [[ "$sibling" == "$self" ]]; then
				continue
			fi
			if ! grep -qF "${sibling} v" "$gm"; then
				continue
			fi

			rel=$(realpath --relative-to="$dir" \
				"${MODULE_DIRS[$sibling]}")
			case "$rel" in
			.*) ;;
			*) rel="./$rel" ;;
			esac

			echo "  - $gm: replace $sibling => $rel"
			go mod edit -replace="$sibling=$rel" "$gm"
		done
	done
}

add_sibling_replaces

for gm in $(go_mods); do
	dir=$(dirname "$gm")

	# Modules that do not depend on btcd at all (e.g. tools modules)
	# only needed the sibling replaces above.
	if ! grep -q 'github.com/btcsuite/btcd' "$gm"; then
		continue
	fi

	# Over-require all new btcd modules at their pinned versions; the
	# final go mod tidy below drops the ones that are not actually
	# imported. Using go get (rather than go mod edit) also bumps the
	# go directive to the 1.25 the new modules require and validates
	# that the pinned tags resolve.
	echo "  - $gm: pinning new btcd v2 modules"
	(cd "$dir" && go get \
		"github.com/btcsuite/btcd@$BTCD_VERSION" \
		"github.com/btcsuite/btcd/btcec/v2@$BTCEC_VERSION" \
		"github.com/btcsuite/btcd/chainhash/v2@$CHAINHASH_VERSION" \
		"github.com/btcsuite/btcd/wire/v2@$WIRE_VERSION" \
		"github.com/btcsuite/btcd/chaincfg/v2@$CHAINCFG_VERSION" \
		"github.com/btcsuite/btcd/address/v2@$ADDRESS_VERSION" \
		"github.com/btcsuite/btcd/txscript/v2@$TXSCRIPT_VERSION" \
		"github.com/btcsuite/btcd/btcutil/v2@$BTCUTIL_VERSION" \
		"github.com/btcsuite/btcd/psbt/v2@$PSBT_VERSION")
done

# ----------------------------------------------------------------------
# Step 5: first tidy pass so goimports resolves against a complete and
# downloaded module graph.
#
# Both tidy passes use -e: cross-repository dependencies (e.g. neutrino
# from btcwallet) are still built against the old btcd module layout and
# import packages like github.com/btcsuite/btcd/wire that no longer
# exist in btcd v0.26. Those dependencies are bumped in manual follow-up
# commits once they have v2-compatible tags, after which tidy runs clean
# again.
# ----------------------------------------------------------------------

echo "==> Running first go mod tidy pass"

for gm in $(go_mods); do
	(cd "$(dirname "$gm")" && go mod tidy -e)
done

# The tidy pass above can have added requirements on sibling modules
# (e.g. for an import that was missing from the go.mod before), so give
# those a local replace directive as well. The final tidy pass below
# picks the replacement up.
add_sibling_replaces

# ----------------------------------------------------------------------
# Step 6: run goimports on all tracked .go files. This removes the
# over-imported address/v2 import (or the now-unused btcutil/v2 import)
# again and normalizes import grouping.
# ----------------------------------------------------------------------

echo "==> Running goimports"

go_files | xargs -r goimports -w

# ----------------------------------------------------------------------
# Step 7: final tidy pass to drop the over-required modules that turned
# out to be unused, and normalize the toolchain directive for a
# reproducible diff across reviewer toolchains.
# ----------------------------------------------------------------------

echo "==> Running final go mod tidy pass"

for gm in $(go_mods); do
	(cd "$(dirname "$gm")" && go mod tidy -e)
	go mod edit -toolchain=none "$gm"
done

# ----------------------------------------------------------------------
# Report.
# ----------------------------------------------------------------------

echo "==> Done. Summary of changes:"
git diff --stat | tail -5

echo ""
echo "==> Build check (failures here are expected to be fixed in"
echo "    follow-up commits, e.g. variables shadowing the new address"
echo "    package or changed APIs):"
build_failed=0
for gm in $(go_mods); do
	dir=$(dirname "$gm")
	if (cd "$dir" && go build ./... >/dev/null 2>&1); then
		echo "  - $dir: OK"
	else
		echo "  - $dir: BUILD FAILED (manual follow-up needed)"
		build_failed=1
	fi
done

if [[ $build_failed -eq 1 ]]; then
	echo ""
	echo "Some modules do not build; inspect with 'go build ./...'"
	echo "in the listed directories and fix in a separate commit."
fi

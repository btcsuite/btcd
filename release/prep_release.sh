#!/bin/sh
#
# Copyright (c) 2013 Conformal Systems LLC <info@conformal.com>
#
# Permission to use, copy, modify, and distribute this software for any
# purpose with or without fee is hereby granted, provided that the above
# copyright notice and this permission notice appear in all copies.
#
# THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
# WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
# MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
# ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
# WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
# ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
# OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
#
#
# Prepares for a release:
#   - Bumps version according to specified level (major, minor, or patch)
#   - Updates all version files and package control files with new version
#   - Performs some basic validation on specified release notes
#   - Updates project changes file with release notes
#

PROJECT=btcd
PROJECT_UC=$(echo $PROJECT | tr '[:lower:]' '[:upper:]')
SCRIPT=$(basename $0)
VERFILE=../version.go
VERFILES="$VERFILE ../cmd/btcctl/version.go"
PROJ_CHANGES=../CHANGES

# verify params
if [ $# -lt 2 ]; then
	echo "usage: $SCRIPT {major | minor | patch} release-notes-file"
	exit 1
fi

CUR_DIR=$(pwd)
cd "$(dirname $0)"

# verify version files exist
for verfile in $VERFILES; do
	if [ ! -f "$verfile" ]; then
		echo "$SCRIPT: error: $verfile does not exist" 1>&2
		exit 1
	fi
done

# verify changes file exists
if [ ! -f "$PROJ_CHANGES" ]; then
	echo "$SCRIPT: error: $PROJ_CHANGES does not exist" 1>&2
	exit 1
fi

RTYPE="$1"
RELEASE_NOTES="$2"
if [ $(echo $RELEASE_NOTES | cut -c1) != "/" ]; then
	RELEASE_NOTES="$CUR_DIR/$RELEASE_NOTES"
fi

# verify valid release type
if [ "$RTYPE" != "major" -a "$RTYPE" != "minor" -a "$RTYPE" != "patch" ]; then
	echo "$SCRIPT: error: release type must be major, minor, or patch"
	exit 1
fi

# verify release notes
if [ ! -e "$RELEASE_NOTES" ]; then
	echo "$SCRIPT: error: specified release notes file does not exist"
	exit 1
fi

if [ ! -s "$RELEASE_NOTES" ]; then
	echo "$SCRIPT: error: specified release notes file is empty"
	exit 1
fi

# verify release notes format
while IFS='' read line; do
	if [ -z "$line" ]; then
		echo "$SCRIPT: error: release notes must not have blank lines"
		exit 1
	fi
	if [ ${#line} -gt 74 ]; then
		echo -n "$SCRIPT: error: release notes must not contain lines "
		echo    "with more than 74 characters"
		exit 1
	fi
	if expr "$line" : ".*\.$" >/dev/null 2>&1 ; then
		echo -n "$SCRIPT: error: release notes must not contain lines "
		echo    "that end in a period"
		exit 1
	fi
	if ! expr "$line" : "\-" >/dev/null 2>&1; then
	if ! expr "$line" : "  " >/dev/null 2>&1; then
		echo -n "$SCRIPT: error: release notes must not contain lines "
		echo    "that do not begin with a dash and are not indented"
		exit 1
	fi
	fi
done <"$RELEASE_NOTES"

# verify git is available
if ! type git >/dev/null 2>&1; then
	echo -n "$SCRIPT: error: Unable to find 'git' in the system path."
	exit 1
fi

# verify the git repository is on the master branch
BRANCH=$(git branch | grep '\*' | cut -c3-)
if [ "$BRANCH" != "master" ]; then
	echo "$SCRIPT: error: git repository must be on the master branch."
	exit 1
fi

# verify there are no uncommitted modifications prior to release modifications
NUM_MODIFIED=$(git diff 2>/dev/null | wc -l | sed 's/^[ \t]*//')
NUM_STAGED=$(git diff --cached 2>/dev/null | wc -l | sed 's/^[ \t]*//')
if [ "$NUM_MODIFIED" != "0" -o "$NUM_STAGED" != "0" ]; then
	echo -n "$SCRIPT: error: the working directory contains uncommitted "
	echo    "modifications"
	exit 1
fi

# get version
PAT_PREFIX="(^[[:space:]]+app"
PAT_SUFFIX='[[:space:]]+uint[[:space:]]+=[[:space:]]+)[0-9]+$'
MAJOR=$(egrep "${PAT_PREFIX}Major${PAT_SUFFIX}" $VERFILE | awk '{print $4}')
MINOR=$(egrep "${PAT_PREFIX}Minor${PAT_SUFFIX}" $VERFILE | awk '{print $4}')
PATCH=$(egrep "${PAT_PREFIX}Patch${PAT_SUFFIX}" $VERFILE | awk '{print $4}')
if [ -z "$MAJOR" -o -z "$MINOR" -o -z "$PATCH" ]; then
	echo "$SCRIPT: error: unable to get version from $VERFILE" 1>&2
	exit 1
fi

# bump version according to level
if [ "$RTYPE" = "major" ]; then
	MAJOR=$(expr $MAJOR + 1)
	MINOR=0
	PATCH=0
elif [ "$RTYPE" = "minor" ]; then
	MINOR=$(expr $MINOR + 1)
	PATCH=0
elif [ "$RTYPE" = "patch" ]; then
	PATCH=$(expr $PATCH + 1)
fi
PROJ_VER="$MAJOR.$MINOR.$PATCH"

# update project changes with release notes
DATE=$(date "+%a %b %d %Y")
awk -v D="$DATE" -v VER="$PROJ_VER" '
/=======/ && first_line==0 {
	first_line=1
	print $0
	next
}
/=======/ && first_line==1 {
	print $0
	print ""
	print "Changes in "VER" ("D")"
	exit
}
{ print $0 }
' <"$PROJ_CHANGES" >"${PROJ_CHANGES}.tmp"
cat "$RELEASE_NOTES" | sed 's/^/  /' >>"${PROJ_CHANGES}.tmp"
awk '
/=======/ && first_line==0 {
	first_line=1
	next
}
/=======/ && first_line==1 {
	second_line=1
	next
}
second_line==1 { print $0 }
' <"$PROJ_CHANGES" >>"${PROJ_CHANGES}.tmp"

# update version filef with new version
for verfile in $VERFILES; do
	sed -E "
	    s/${PAT_PREFIX}Major${PAT_SUFFIX}/\1${MAJOR}/;
	    s/${PAT_PREFIX}Minor${PAT_SUFFIX}/\1${MINOR}/;
	    s/${PAT_PREFIX}Patch${PAT_SUFFIX}/\1${PATCH}/;
	" <"$verfile" >"${verfile}.tmp"
done


# Apply changes
mv "${PROJ_CHANGES}.tmp" "$PROJ_CHANGES"
for verfile in $VERFILES; do
	mv "${verfile}.tmp" "$verfile"
done

echo "All files have been prepared for release."
echo "Use the following commands to review the changes for accuracy:"
echo "  git status"
echo "  git diff"
echo ""
echo "If everything is accurate, use the following commands to commit, tag,"
echo "and push the changes"
echo "  git commit -am \"Prepare for release ${PROJ_VER}.\""
echo -n "  git tag -a \"${PROJECT_UC}_${MAJOR}_${MINOR}_${PATCH}\" -m "
echo    "\"Release ${PROJ_VER}\""
echo "  git push"
echo "  git push --tags"

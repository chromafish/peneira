#!/bin/sh
# release-notes.sh <tag> — the release description for a tag.
set -eu

tag=${1:?usage: release-notes.sh <tag>}

to=$(git rev-parse "$tag^{commit}")
prev=$(git describe --tags --abbrev=0 "$tag^" 2>/dev/null || true)

if [ -n "$prev" ]; then
	range="$(git rev-parse "$prev^{commit}")..${to}"
	printf '## Changes since %s\n\n' "$prev"
else
	range="$to"
	printf '## Changes\n\n'
fi

# Oldest first, one line per commit, skipping any with an empty subject.
git log --reverse --format='%s' "$range" | sed '/^$/d; s/^/- /'

cat <<'NOTE'

## Installing

Download the `.zip`, unzip it, and drag `Peneira.app` to `/Applications`. The
build is not signed or notarised, so the first launch needs either a
right-click → Open, or:

    xattr -dr com.apple.quarantine /Applications/Peneira.app

The `.tar.gz` holds the same binary for `~/.local/bin`.
NOTE

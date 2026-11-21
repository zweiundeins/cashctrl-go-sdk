#!/usr/bin/env bash
# Pulls spec/api.json and spec/responses.json from a tag or commit of the
# TypeScript SDK, records it in spec/SOURCE and regenerates the client.
#
#   scripts/sync-spec.sh v0.6.0
#   scripts/sync-spec.sh 24b5e3b /path/to/cashctrl-ts-sdk   # a local clone
#
# Without a local clone it fetches the files from GitHub. It warns when the
# sync moves the TypeScript SDK's scripts/overrides.ts, whose corrections
# internal/gen/overrides.go ports by hand.
set -euo pipefail

ref=${1:?usage: scripts/sync-spec.sh <tag-or-commit> [local clone of cashctrl-ts-sdk]}
clone=${2:-}
repo=https://github.com/zweiundeins/cashctrl-ts-sdk
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -n "$clone" ]; then
	commit=$(git -C "$clone" rev-parse --verify "$ref^{commit}")
	for f in spec/api.json spec/responses.json scripts/overrides.ts; do
		git -C "$clone" show "$commit:$f" >"$tmp/$(basename "$f")"
	done
else
	refs=$(git ls-remote "$repo" "refs/tags/$ref" "refs/tags/$ref^{}" "refs/heads/$ref")
	# An annotated tag lists the tag object first; the commit is the peeled line.
	commit=$(awk '$2 ~ /\^\{\}$/ {print $1; exit}' <<<"$refs")
	if [ -z "$commit" ]; then
		commit=$(head -n1 <<<"$refs" | cut -f1)
	fi
	if [ -z "$commit" ]; then
		commit=$ref # a commit hash, which ls-remote does not list
	fi
	for f in spec/api.json spec/responses.json scripts/overrides.ts; do
		curl -fsSL "https://raw.githubusercontent.com/zweiundeins/cashctrl-ts-sdk/$commit/$f" -o "$tmp/$(basename "$f")"
	done
fi

for f in api.json responses.json; do
	python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$tmp/$f" || {
		echo "sync-spec: $f at $ref is not JSON" >&2
		exit 1
	}
done

sha() { sha256sum "$1" | cut -d' ' -f1; }
old_overrides=$(sed -n 's/^overrides.ts.sha256=//p' spec/SOURCE 2>/dev/null || true)
new_overrides=$(sha "$tmp/overrides.ts")

cp "$tmp/api.json" "$tmp/responses.json" spec/
cat >spec/SOURCE <<EOF
# Where spec/api.json and spec/responses.json come from. Written by
# scripts/sync-spec.sh; do not edit by hand.
repo=$repo
ref=$ref
commit=$commit
api.json.sha256=$(sha spec/api.json)
responses.json.sha256=$(sha spec/responses.json)
overrides.ts.sha256=$new_overrides
EOF

go run ./cmd/gen -spec spec -out .
gofmt -l . | (! grep .) || {
	echo "sync-spec: gofmt reports files" >&2
	exit 1
}
go vet ./...
go test ./...

if [ "$old_overrides" != "$new_overrides" ]; then
	echo
	echo "sync-spec: scripts/overrides.ts changed in the TypeScript SDK. Port the change to"
	echo "internal/gen/overrides.go, regenerate and test again before committing:"
	echo "  $repo/blob/$commit/scripts/overrides.ts"
fi
echo
git status --short spec/ ./*_gen.go ./*_gen_test.go

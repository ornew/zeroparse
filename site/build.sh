#!/bin/sh
# Project theme and special pages use the pinned ZeroSite extension APIs.
set -eu
cd "$(dirname "$0")"
revision=$(cat zerosite.rev)
case "$revision" in *[!0-9a-f]*|'') echo 'Invalid ZeroSite commit pin.' >&2; exit 1;; esac
[ "${#revision}" -eq 40 ] || { echo 'Use a full ZeroSite commit SHA.' >&2; exit 1; }
expected=$(GOWORK=off go list -m -f '{{.Version}}' "github.com/ornew/zerosite@$revision")
replacement=$(GOWORK=off go list -m -f '{{if .Replace}}{{.Replace.Path}}{{end}}' github.com/ornew/zerosite)
[ -z "$replacement" ] || { echo 'Pinned ZeroSite dependency must not use a module replacement.' >&2; exit 1; }
actual=$(GOWORK=off go list -m -f '{{.Version}}' github.com/ornew/zerosite)
[ "$actual" = "$expected" ] || {
  echo 'ZeroSite dependency differs from site/zerosite.rev; run hack/update-zerosite-skill.sh.' >&2
  exit 1
}
GOWORK=off exec go run . -repo .. -out dist "$@"

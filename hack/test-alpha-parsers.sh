#!/bin/sh
# Run the tests of the alpha-parsers checkout against this working tree.
#
# alpha-parsers (zeroparse.dev/alpha-parsers) holds the ready-made parsers, generated from the grammars of
# testdata/grammars/, which are the source of truth: the script first checks that every grammar, every sample
# input and duckdb/inspect.go of the checkout equals this tree's copy, and fails when one differs. It is a separate repository, so a change of the generator or the engine is checked
# against it from a checkout next to this one, joined by a go.work file:
#
#	hack/test-alpha-parsers.sh [go test arguments]
#
# $ZP_ALPHA_PARSERS names the checkout (default: ../alpha-parsers). The script does nothing, and succeeds,
# when there is no checkout, so that it can stay in a routine check list.
set -u
root=$(cd "$(dirname "$0")/.." && pwd) || exit 1
dir=${ZP_ALPHA_PARSERS:-$root/../alpha-parsers}
if [ ! -f "$dir/test.sh" ]; then
	echo "alpha-parsers: no checkout at $dir (set ZP_ALPHA_PARSERS); skipped" >&2
	exit 0
fi
dir=$(cd "$dir" && pwd) || exit 1
# Sync check: testdata/grammars/<n>/ here against <n>/ (grammar, duckdb/inspect.go) and <n>/testdata/ (inputs) there.
bad=0
for d in "$root"/testdata/grammars/*/; do
	n=$(basename "$d")
	for f in "$d"*.zp "$d"inspect.go; do
		[ -f "$f" ] || continue
		cmp -s "$f" "$dir/$n/$(basename "$f")" || { echo "alpha-parsers: $n/$(basename "$f") differs from testdata/grammars/$n" >&2; bad=1; }
	done
	for f in "$d"*.txt; do
		[ -f "$f" ] || continue
		cmp -s "$f" "$dir/$n/testdata/$(basename "$f")" || { echo "alpha-parsers: $n/testdata/$(basename "$f") differs from testdata/grammars/$n" >&2; bad=1; }
	done
	for f in "$dir/$n/testdata/"*.txt; do
		[ -f "$f" ] || continue
		[ -f "$d$(basename "$f")" ] || { echo "alpha-parsers: $n/testdata/$(basename "$f") is not in testdata/grammars/$n" >&2; bad=1; }
	done
done
[ "$bad" = 0 ] || { echo "alpha-parsers: copy the grammars from this tree (testdata/grammars is the source of truth)" >&2; exit 1; }
work=$(mktemp -d) || exit 1
trap 'rm -rf "$work"' EXIT
# A go.work file outside both trees joins the main module and the alpha-parsers module.
printf 'go 1.27.1\n\nuse (\n\t%s\n\t%s\n)\n' "$root" "$dir" >"$work/go.work"
cd "$dir" || exit 1
GOWORK="$work/go.work" ./test.sh "$@"

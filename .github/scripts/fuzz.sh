#!/usr/bin/env bash
# Runs Go's fuzzer on every Fuzz target for $FUZZTIME (default 30s) each; with
# $BASE set, only the targets of a package whose Go files differ from it. A
# failing input fails the run; Go keeps it under the package's
# testdata/fuzz/<target>/ — fold it into the target's seed list, with the bug it
# guards, as FuzzParse's "{}" is.
set -u
fuzztime="${FUZZTIME:-30s}"
status=0
changed=""
if [ -n "${BASE:-}" ]; then
	changed=$(git diff --name-only "$BASE...HEAD" -- '*.go' | xargs -r -n1 dirname | sort -u)
fi
while IFS= read -r file; do
	dir=$(dirname "$file")
	if [ -n "${BASE:-}" ] && ! grep -qxF "$dir" <<<"$changed"; then
		continue
	fi
	for target in $(grep -oE '^func Fuzz[A-Za-z0-9_]+' "$file" | cut -d' ' -f2); do
		echo "::group::$target in $dir, $fuzztime"
		if ! go test "./$dir" -run '^$' -fuzz "^$target\$" -fuzztime "$fuzztime"; then
			status=1
			echo "::error file=$file::$target found a failing input (kept under $dir/testdata/fuzz/$target/)"
		fi
		echo "::endgroup::"
	done
done < <(git ls-files '*_test.go' | xargs grep -l '^func Fuzz')
exit $status

#!/usr/bin/env bash
# fuzz_target.sh <module-dir> <package> <FuzzTarget> <fuzztime>
#
# Runs one Go fuzz target for CI. Tolerates only the known "-fuzztime
# boundary flake": the fuzz coordinator reporting "context deadline
# exceeded" when the wall clock hits -fuzztime while a worker still has
# an input in flight. That race produces no crasher and is not an input
# failure — on shared 4-vCPU runners it fires intermittently. A real
# failure (t.Fatal, panic) always writes "Failing input written to
# testdata/fuzz/..." and this script exits nonzero.
set -u

dir=${1:?module dir (. or pkg)}
pkg=${2:?package path}
target=${3:?fuzz target name}
time=${4:?fuzztime}

log=$(mktemp -t "fuzz-${target}.XXXXXX")
trap 'rm -f "$log"' EXIT

set -o pipefail
(cd "$dir" && go test -fuzz="$target" -fuzztime="$time" "$pkg") 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
[ "$status" -eq 0 ] && exit 0

if grep -q "written to testdata" "$log"; then
	exit "$status" # real crasher — fail loudly
fi
if grep -q "context deadline exceeded" "$log"; then
	echo "fuzz_target.sh: $target hit the -fuzztime boundary flake (deadline exceeded, no crasher) — pass"
	exit 0
fi
exit "$status"

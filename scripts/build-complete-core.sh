#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"
cd "$repo_root"

if ! command -v go >/dev/null 2>&1; then
  echo "error: Go is required to build the Complete core" >&2
  exit 2
fi
if ! command -v git >/dev/null 2>&1; then
  echo "error: git is required to identify the Complete core build" >&2
  exit 2
fi

output="${1:-${repo_root}/bin/verge-mihomo-tt-x86_64-unknown-linux-gnu}"
case "$output" in
  /*) ;;
  *) output="${repo_root}/${output}" ;;
esac
mkdir -p "$(dirname -- "$output")"

temp_output="$(mktemp "${output}.tmp.XXXXXX")"
cleanup() {
  rm -f -- "$temp_output"
}
trap cleanup EXIT

commit="$(git rev-parse HEAD)"
short_commit="$(git rev-parse --short=12 HEAD)"
source_epoch="${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct HEAD)}"
if [[ ! "$source_epoch" =~ ^[0-9]+$ ]]; then
  echo "error: SOURCE_DATE_EPOCH must be an integer Unix timestamp" >&2
  exit 2
fi
build_time="$(date -u --date="@${source_epoch}" '+%Y-%m-%dT%H:%M:%SZ')"
version="traffictracer-complete-${short_commit}"
ldflags="-X 'github.com/metacubex/mihomo/constant.Version=${version}' -X 'github.com/metacubex/mihomo/constant.BuildTime=${build_time}' -w -s -buildid="

echo "Building ${version} (${commit})..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 \
  go build -tags with_gvisor -trimpath -buildvcs=false -ldflags "$ldflags" -o "$temp_output" .
chmod 0755 "$temp_output"
mv -f -- "$temp_output" "$output"
trap - EXIT

printf 'Complete core: %s\n' "$output"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$output"
fi

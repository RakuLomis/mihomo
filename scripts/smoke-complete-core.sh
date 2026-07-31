#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"
binary="${1:-${repo_root}/bin/verge-mihomo-tt-x86_64-unknown-linux-gnu}"
case "$binary" in
  /*) ;;
  *) binary="${repo_root}/${binary}" ;;
esac

if [[ ! -x "$binary" ]]; then
  echo "error: Complete core is not executable: $binary" >&2
  echo "run scripts/build-complete-core.sh first" >&2
  exit 2
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required for the Unix Socket smoke test" >&2
  exit 2
fi
if ! command -v python >/dev/null 2>&1; then
  echo "error: Python is required to validate the capabilities response" >&2
  exit 2
fi

smoke_dir="$(mktemp -d)"
socket_path="${smoke_dir}/mihomo.sock"
config_path="${smoke_dir}/config.yaml"
log_path="${smoke_dir}/mihomo.log"
core_pid=""
cleanup() {
  if [[ -n "$core_pid" ]] && kill -0 "$core_pid" 2>/dev/null; then
    kill "$core_pid" 2>/dev/null || true
    wait "$core_pid" 2>/dev/null || true
  fi
  rm -rf -- "$smoke_dir"
}
trap cleanup EXIT INT TERM

tee "$config_path" >/dev/null <<CONFIG
mixed-port: 0
allow-lan: false
mode: direct
log-level: silent
ipv6: false
external-controller-unix: ${socket_path}
secret: ""
proxies: []
proxy-groups: []
rules: []
CONFIG

"$binary" -t -d "$smoke_dir" -f "$config_path"
"$binary" -d "$smoke_dir" -f "$config_path" >"$log_path" 2>&1 &
core_pid=$!

response=""
for _ in {1..50}; do
  if ! kill -0 "$core_pid" 2>/dev/null; then
    echo "error: Complete core exited during startup" >&2
    sed -n '1,160p' "$log_path" >&2
    exit 3
  fi
  if [[ -S "$socket_path" ]]; then
    response="$(curl --silent --show-error --fail --unix-socket "$socket_path" http://localhost/experimental/tracing/capabilities)" && break
  fi
  sleep 0.1
done
if [[ -z "$response" ]]; then
  echo "error: tracing capabilities endpoint was not ready" >&2
  sed -n '1,160p' "$log_path" >&2
  exit 3
fi

python -c '
import json
import sys
payload = json.loads(sys.argv[1])
assert payload["api_version"] == 1
assert payload["event_schema_version"] == 1
for key in (
    "supports_tcp",
    "supports_udp",
    "supports_normalized_flow",
    "supports_outer_conn_id",
    "supports_session_id",
    "supports_shared_outer_flow",
):
    assert payload[key] is True, key
' "$response"
printf 'Capabilities: %s\n' "$response"
echo "Complete core smoke test passed."

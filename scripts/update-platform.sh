#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: scripts/update-platform.sh [--check] [--force] [beeper]"
}

check_only=""
force=""
platform=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) check_only=1 ;;
    --force) force=1 ;;
    -h|--help) usage; exit 0 ;;
    -*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$platform" ]]; then
        echo "Unexpected extra argument: $1" >&2
        usage >&2
        exit 2
      fi
      platform="$1"
      ;;
  esac
  shift
done

platform="${platform:-beeper}"
if [[ "$platform" != "beeper" ]]; then
  echo "Unsupported platform: $platform (only 'beeper' is supported)." >&2
  exit 2
fi

cd "$(dirname "$(realpath "$0")")/.."
root="$PWD"
common="$(dirname "$(realpath "$(git rev-parse --git-common-dir)")")"
if [[ "$root" != "$common" ]]; then
  echo "Update platform from the main checkout ($common), not a worktree." >&2
  exit 1
fi

go="$(go env GOROOT)/bin/go"
if ! jq="$(command -v jq)"; then
  echo "jq is missing. Run mise install." >&2
  exit 1
fi
if ! curl_bin="$(command -v curl)"; then
  echo "curl is missing." >&2
  exit 1
fi
if ! openssl_bin="$(command -v openssl)"; then
  echo "openssl is missing." >&2
  exit 1
fi

config="${GENTALENT_CONFIG:-$root/config.json}"
opt_dir="${BEEPER_OPT_DIR:-$HOME/.local/opt/beeper}"
cache_dir="${BEEPER_UPDATER_CACHE_DIR:-$HOME/.cache/beepertexts-updater/pending}"
download_url="${BEEPER_DOWNLOAD_URL:-https://api.beeper.com/desktop/download/linux/x64/stable/com.automattic.beeper.desktop}"
api_url="${BEEPER_API_URL:-http://127.0.0.1:23373}"
stop_timeout="${BEEPER_STOP_TIMEOUT:-20}"
ready_timeout="${BEEPER_READY_TIMEOUT:-120}"
export BEEPER_UPDATER_CACHE_DIR="$cache_dir"

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

if [[ -n "${GENTALENT_BIN:-}" && -x "${GENTALENT_BIN}" ]]; then
  bot_bin="$GENTALENT_BIN"
else
  bot_bin="$scratch/gentalent"
  "$go" build -o "$bot_bin" ./cmd/gentalent
fi

running_bot="$bot_bin"
if [[ -x .live/bin/gentalent ]]; then
  running_bot="$root/.live/bin/gentalent"
fi

if ! "$running_bot" --config "$config" busy --live; then
  echo "The live bot is mid-operation (an AI search is running). Try again after it finishes." >&2
  exit 1
fi

extract_appimage_version() {
  local name="$1"
  if [[ "$name" != Beeper-*.AppImage ]]; then
    return 1
  fi
  local trimmed="${name#Beeper-}"
  trimmed="${trimmed%.AppImage}"
  local suffix
  for suffix in -x86_64 -aarch64 -arm64 -amd64 -x64; do
    if [[ "$trimmed" == *"$suffix" ]]; then
      trimmed="${trimmed%"$suffix"}"
      break
    fi
  done
  printf '%s\n' "$trimmed"
}

verify_sha512() {
  local file="$1" expected="$2"
  local expected_trimmed="${expected//[[:space:]]/}"
  if [[ ${#expected_trimmed} -eq 128 && "$expected_trimmed" =~ ^[0-9a-fA-F]+$ ]]; then
    local actual_hex
    actual_hex="$("$openssl_bin" dgst -sha512 -r "$file" | awk '{print $1}')"
    [[ "${actual_hex,,}" == "${expected_trimmed,,}" ]]
    return $?
  fi
  local actual_b64
  actual_b64="$("$openssl_bin" dgst -sha512 -binary "$file" | base64 | tr -d '\n')"
  [[ "$actual_b64" == "$expected_trimmed" || "${actual_b64%%=*}" == "${expected_trimmed%%=*}" ]]
}

swap_symlink() {
  local target="$1" link="$2"
  local tmp="${link}.new.$$"
  ln -s "$target" "$tmp"
  mv -Tf "$tmp" "$link"
}

is_beeper_down() {
  if "$curl_bin" -sf "${api_url}/v1/info" >/dev/null 2>&1; then
    return 1
  fi
  if [[ -z "${BEEPER_SKIP_PGREP:-}" ]] && pgrep -x "${BEEPER_PROCESS_NAME:-beepertexts}" >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

restart_beeper() {
  timeout 60 mise daemons stop beeper >/dev/null 2>&1 || true
  local stop_waited=0
  while (( stop_waited < stop_timeout )); do
    if is_beeper_down; then
      break
    fi
    sleep 1
    stop_waited=$((stop_waited + 1))
  done
  timeout 120 mise daemons start beeper >/dev/null 2>&1 || true
  local waited=0
  while (( waited < ready_timeout )); do
    if "$curl_bin" -sf "${api_url}/v1/info" >/dev/null 2>&1 && \
       "$curl_bin" -sf "${api_url}/v1/spec" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

check_rc=0
check_json="$("$bot_bin" --config "$config" check-updates --platform beeper --live --json --cache-dir "$cache_dir" 2>"$scratch/check.err")" || check_rc=$?
if [[ -z "$check_json" ]] || ! printf '%s' "$check_json" | "$jq" -e '.' >/dev/null 2>&1; then
  cat "$scratch/check.err" >&2
  exit 1
fi

current_version="$(printf '%s' "$check_json" | "$jq" -r '.current_version // empty')"
has_pending="$(printf '%s' "$check_json" | "$jq" -r '.pending != null')"

candidate_source=""
candidate_version=""
candidate_filename=""
candidate_src_path=""
candidate_sha512=""

if [[ "$has_pending" == "true" ]]; then
  pending_verified="$(printf '%s' "$check_json" | "$jq" -r '.pending.verified // false')"
  if [[ "$pending_verified" != "true" ]]; then
    pending_err="$(printf '%s' "$check_json" | "$jq" -r '.pending.verification_error // "pending update failed verification"')"
    echo "Refusing pending update: $pending_err" >&2
    exit 1
  fi
  candidate_source="cache"
  candidate_version="$(printf '%s' "$check_json" | "$jq" -r '.pending.version // empty')"
  candidate_filename="$(printf '%s' "$check_json" | "$jq" -r '.pending.file_name // empty')"
  candidate_src_path="$(printf '%s' "$check_json" | "$jq" -r '.pending.file_path // empty')"
  candidate_sha512="$(printf '%s' "$check_json" | "$jq" -r '.pending.sha512 // empty')"
  if [[ -z "$candidate_filename" || "$candidate_filename" == *"/"* || "$candidate_filename" == ".." || ! -f "$candidate_src_path" ]]; then
    echo "Pending update file is invalid or missing: $candidate_filename" >&2
    exit 1
  fi
elif [[ -n "$check_only" ]]; then
  echo "Check only: no pending update cached in $cache_dir (current version: ${current_version:-unknown}); would download from $download_url without --check."
  exit 0
else
  candidate_source="download"
  effective_url="$("$curl_bin" -fSL --dump-header "$scratch/download.headers" -o "$scratch/downloaded.AppImage" -w '%{url_effective}' "$download_url")"
  header_name="$(grep -i '^content-disposition:' "$scratch/download.headers" 2>/dev/null | sed -n 's/.*filename="\?\([^";\r\n]*\)"\?.*/\1/p' | tail -n 1 || true)"
  if [[ -n "$header_name" ]]; then
    candidate_filename="$(basename "$header_name")"
  else
    clean_url="${effective_url%%\?*}"
    candidate_filename="$(basename "$clean_url")"
  fi
  if [[ -z "$candidate_filename" || "$candidate_filename" == *"/"* || "$candidate_filename" == ".." || "$candidate_filename" != Beeper-*.AppImage ]]; then
    echo "Downloaded filename $candidate_filename does not match Beeper-*.AppImage." >&2
    exit 1
  fi
  candidate_version="$(extract_appimage_version "$candidate_filename" || true)"
  if [[ -z "$candidate_version" ]]; then
    echo "Could not determine version from downloaded filename $candidate_filename." >&2
    exit 1
  fi
  candidate_src_path="$scratch/downloaded.AppImage"
fi

assess_args=(platform-update assess --platform beeper --candidate "$candidate_version" --live --json --cache-dir "$cache_dir")
if [[ -n "$force" ]]; then
  assess_args+=(--force)
fi
assess_rc=0
assess_json="$("$bot_bin" --config "$config" "${assess_args[@]}" 2>"$scratch/assess.err")" || assess_rc=$?
if [[ -z "$assess_json" ]] || ! printf '%s' "$assess_json" | "$jq" -e '.' >/dev/null 2>&1; then
  cat "$scratch/assess.err" >&2
  exit 1
fi
allowed="$(printf '%s' "$assess_json" | "$jq" -r '.managed_update_allowed // false')"
if [[ "$allowed" != "true" || "$assess_rc" -ne 0 ]]; then
  refusal="$(printf '%s' "$assess_json" | "$jq" -r '.refusal_reason // empty')"
  echo "${refusal:-Managed update refused for $candidate_version.}" >&2
  exit 1
fi

if [[ -z "$force" && -n "$current_version" && "$candidate_version" == "$current_version" ]]; then
  echo "Platform beeper is already running version $current_version."
  exit 0
fi

if [[ -n "$check_only" ]]; then
  echo "Check only: would update beeper ${current_version:+from $current_version }to $candidate_version (source: $candidate_source${force:+, forced})."
  exit 0
fi

previous_target=""
if [[ -L "$opt_dir/current.AppImage" || -e "$opt_dir/current.AppImage" ]]; then
  previous_target="$(realpath "$opt_dir/current.AppImage" 2>/dev/null || true)"
elif [[ -d "$opt_dir" ]]; then
  previous_target="$(find "$opt_dir" -maxdepth 1 -type f -name 'Beeper-*.AppImage' -printf '%f\t%p\n' 2>/dev/null | sort -V | tail -n 1 | cut -f2- || true)"
fi

previous_basename=""
if [[ -n "$previous_target" ]]; then
  previous_basename="$(basename "$previous_target")"
fi
previous_version="$current_version"
if [[ -z "$previous_version" && -n "$previous_basename" ]]; then
  previous_version="$(extract_appimage_version "$previous_basename" || true)"
fi

mkdir -p "$opt_dir"
tmp_install="$opt_dir/.${candidate_filename}.new.$$"
cp "$candidate_src_path" "$tmp_install"
chmod 755 "$tmp_install"
if [[ -n "$candidate_sha512" ]]; then
  if ! verify_sha512 "$tmp_install" "$candidate_sha512"; then
    rm -f "$tmp_install"
    echo "Copied candidate $candidate_filename failed SHA-512 verification." >&2
    exit 1
  fi
fi
mv -f "$tmp_install" "$opt_dir/$candidate_filename"

if [[ -n "$previous_basename" && "$previous_basename" != "$candidate_filename" && -e "$opt_dir/$previous_basename" ]]; then
  swap_symlink "$previous_basename" "$opt_dir/previous.AppImage"
fi
swap_symlink "$candidate_filename" "$opt_dir/current.AppImage"

api_ready=1
if ! restart_beeper; then
  api_ready=0
fi

verify_ok=1
verify_reason=""
verify_json=""
if [[ "$api_ready" -eq 0 ]]; then
  verify_ok=0
  verify_reason="Beeper API did not become ready on $api_url within ${ready_timeout}s after update"
else
  verify_rc=0
  verify_json="$("$bot_bin" --config "$config" platform-update verify --platform beeper --previous "$previous_version" --target "$candidate_version" --live --json --cache-dir "$cache_dir" 2>"$scratch/verify.err")" || verify_rc=$?
  if [[ "$verify_rc" -ne 0 ]]; then
    verify_ok=0
    if [[ -n "$verify_json" ]] && printf '%s' "$verify_json" | "$jq" -e '.incompatible_reasons' >/dev/null 2>&1; then
      verify_reason="$(printf '%s' "$verify_json" | "$jq" -r '.incompatible_reasons | join("; ")')"
    else
      verify_reason="$(cat "$scratch/verify.err")"
    fi
  elif ! "$bot_bin" --config "$config" doctor --contract >"$scratch/doctor.out" 2>"$scratch/doctor.err"; then
    verify_ok=0
    verify_reason="$(cat "$scratch/doctor.err")"
  fi
fi

if [[ "$verify_ok" -ne 1 ]]; then
  echo "Platform update to $candidate_version failed compatibility verification: $verify_reason" >&2
  if [[ -n "$previous_basename" && -e "$opt_dir/$previous_basename" ]]; then
    echo "Rolling back current.AppImage to $previous_basename." >&2
    swap_symlink "$previous_basename" "$opt_dir/current.AppImage"
    restart_beeper || true
  fi
  "$bot_bin" --config "$config" platform-update record-rollback \
    --platform beeper \
    --previous "$previous_version" \
    --failed "$candidate_version" \
    --reason "$verify_reason" \
    --json >/dev/null || true
  exit 1
fi

if [[ "$candidate_source" == "cache" ]]; then
  rm -f "$candidate_src_path" "$cache_dir/update-info.json"
fi

review_notice="$(printf '%s' "$verify_json" | "$jq" -r '.review_notice // empty')"
echo "Updated beeper to $candidate_version and verified API contract compatibility."
if [[ -n "$review_notice" ]]; then
  echo "Review note: $review_notice"
fi

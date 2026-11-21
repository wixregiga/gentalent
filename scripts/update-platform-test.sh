#!/usr/bin/env bash
set -euo pipefail

source_root="$(cd "$(dirname "$(realpath "$0")")/.." && pwd)"
go_bin="$(dirname "$(go env GOROOT)/bin/go")"
jq_bin="$(dirname "$(mise which jq)")"
work="$(mktemp -d)"
mock_pid=""

cleanup() {
  if [[ -n "$mock_pid" ]]; then
    kill "$mock_pid" >/dev/null 2>&1 || true
    wait "$mock_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

mkdir -p "$work/stubs" "$work/repo/scripts" "$work/opt" "$work/cache" "$work/state/platforms" "$work/secret"

cp "$source_root/scripts/update-platform.sh" "$work/repo/scripts/update-platform.sh"
cp "$source_root/scripts/run-beeper.sh" "$work/repo/scripts/run-beeper.sh"
chmod +x "$work/repo/scripts/update-platform.sh" "$work/repo/scripts/run-beeper.sh"

git init --quiet -b main "$work/repo"
git -C "$work/repo" config user.name test
git -C "$work/repo" config user.email test@example.invalid
git -C "$work/repo" add -A
git -C "$work/repo" commit --quiet -m "Test repo"

bot_bin="$work/gentalent"
"$go_bin/go" -C "$source_root" build -o "$bot_bin" ./cmd/gentalent

cat >"$work/mock_server.py" <<'PY'
import hashlib
import http.server
import json
import os
import pathlib
import sys

state_file = pathlib.Path(sys.argv[1])
port_file = pathlib.Path(sys.argv[2])
opt_dir = pathlib.Path(sys.argv[3])

def load_state():
    return json.loads(state_file.read_text())

class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        return
    def do_GET(self):
        st = load_state()
        if self.path.startswith("/download/"):
            name = st.get("download_file", "Beeper-4.3.230-x86_64.AppImage")
            payload = f"#!/bin/sh\necho downloaded-{name}\n".encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Disposition", f'attachment; filename="{name}"')
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return

        if st.get("api_down"):
            self.send_response(503)
            self.end_headers()
            return

        ver = st["app_version"]
        if (opt_dir / "current.AppImage").exists():
            resolved = os.path.basename(os.path.realpath(opt_dir / "current.AppImage"))
            if resolved.startswith("Beeper-") and resolved.endswith(".AppImage"):
                trimmed = resolved[len("Beeper-"):-len(".AppImage")]
                for suf in ("-x86_64", "-aarch64", "-arm64", "-amd64", "-x64"):
                    if trimmed.endswith(suf):
                        trimmed = trimmed[:-len(suf)]
                        break
                ver = trimmed

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("X-Beeper-Desktop-Version", ver)
        self.end_headers()

        path_only = self.path.split("?", 1)[0]
        if path_only == "/v1/info":
            body = {
                "app": {"bundle_id": "com.automattic.beeper.desktop", "name": "Beeper", "version": ver},
                "endpoints": {"spec": "/v1/spec"},
                "platform": {"os": "linux"},
                "server": {"status": "running"},
            }
        elif path_only == "/v1/accounts":
            body = [{"accountID": "demo-account", "network": "SMS", "status": "connected", "bridge": {"id": "b", "type": "sms", "provider": "local"}, "user": {"id": "alex", "fullName": "Alex", "username": "alex", "isSelf": True}}]
        elif path_only == "/v1/chats/group%2Fone/messages/739":
            body = {"id": "739", "chatID": "group/one", "accountID": "demo-account", "senderID": "alex", "timestamp": "2026-11-21T15:00:00Z", "sortKey": "1", "type": "TEXT", "text": "<p>@gentalent-bot says: ok</p>", "isSender": True}
        elif path_only == "/v1/chats/group%2Fone/messages":
            body = {"items": [], "hasMore": False, "newestCursor": "cursor-1"}
        elif path_only == "/v1/chats/group%2Fone":
            body = {"id": "group/one", "accountID": "demo-account", "type": "group", "participants": {"hasMore": False, "items": [{"id": "blair"}, {"id": "casey"}], "total": 2}}
        elif path_only == "/v1/spec":
            spec_ver = st["spec_version"]
            if ver in st.get("spec_by_version", {}):
                spec_ver = st["spec_by_version"][ver]
            statuses = ["PENDING", "SUCCESS", "FAIL_RETRIABLE", "FAIL_PERMANENT"]
            if ver in st.get("broken_versions", []):
                statuses = ["SUCCESS"]
            body = {
                "info": {"version": spec_ver},
                "components": {"schemas": {"SendStatus": {"properties": {"status": {"enum": statuses}}}, "Message": {"required": ["id"]}}},
                "paths": {"/v1/chats/{chatID}/messages/{messageID}": {"get": {"description": "Retrieve by pendingMessageID"}}},
            }
        else:
            body = {}
        self.wfile.write(json.dumps(body).encode())

srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
port_file.write_text(str(srv.server_address[1]))
srv.serve_forever()
PY

cat >"$work/server_state.json" <<'JSON'
{
  "app_version": "4.3.200",
  "spec_version": "5.0.0",
  "spec_by_version": {},
  "broken_versions": [],
  "api_down": false
}
JSON

python3 "$work/mock_server.py" "$work/server_state.json" "$work/server.port" "$work/opt" &
mock_pid=$!
for _ in $(seq 1 50); do
  [[ -s "$work/server.port" ]] && break
  sleep 0.1
done
port="$(cat "$work/server.port")"
api_url="http://127.0.0.1:$port"

printf 'test-token\n' >"$work/secret/beeper.token"
chmod 600 "$work/secret/beeper.token"
cat >"$work/config.json" <<JSON
{
  "beeper_url": "$api_url",
  "chat_id": "group/one",
  "account_id": "demo-account",
  "owner_id": "alex",
  "members": [
    {"id": "alex", "name": "Alex", "platform": "android"},
    {"id": "blair", "name": "Blair", "platform": "iphone"},
    {"id": "casey", "name": "Casey", "platform": "unknown"}
  ],
  "token_file": "$work/secret/beeper.token",
  "media_dir": "$work/state/media",
  "nominations_database": "$work/state/nominations.sqlite3",
  "messaging_database": "$work/state/messaging.sqlite3"
}
JSON

"$bot_bin" --config "$work/config.json" arm --live >/dev/null
python3 - "$work/state/messaging.sqlite3" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
conn.execute(
    "INSERT INTO outbox (key, text, attachment, attachment_name, tag, kind, status, pending_id, final_id) VALUES (?, ?, '', '', '', 'text', 'sent', 'p-1', '739')",
    ("reply-1", "@gentalent-bot says: ok"),
)
conn.commit()
conn.close()
PY

cat >"$work/stubs/mise" <<STUB
#!/bin/sh
echo "\$*" >>"$work/mise.log"
exit 0
STUB
chmod +x "$work/stubs/mise"

export PATH="$work/stubs:$go_bin:$jq_bin:/usr/bin:/bin"
export GENTALENT_CONFIG="$work/config.json"
export GENTALENT_BIN="$bot_bin"
export BEEPER_OPT_DIR="$work/opt"
export BEEPER_UPDATER_CACHE_DIR="$work/cache"
export BEEPER_DOWNLOAD_URL="$api_url/download/Beeper-4.3.230-x86_64.AppImage"
export BEEPER_API_URL="$api_url"
export BEEPER_READY_TIMEOUT="5"
export GENTALENT_PLATFORM_STATE_DIR="$work/state/platforms"

make_appimage() {
  local dir="$1" version="$2"
  local file="$dir/Beeper-${version}-x86_64.AppImage"
  printf '#!/bin/sh\necho version-%s\n' "$version" >"$file"
  chmod 755 "$file"
  printf '%s' "$file"
}

write_cache_update() {
  local version="$1" corrupt="${2:-}"
  local file
  file="$(make_appimage "$work/cache" "$version")"
  local digest
  if [[ -n "$corrupt" ]]; then
    digest="$(printf 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==' )"
  else
    digest="$(openssl dgst -sha512 -binary "$file" | base64 | tr -d '\n')"
  fi
  printf '{"fileName":"%s","sha512":"%s","isAdminRightsRequired":false}\n' "$(basename "$file")" "$digest" >"$work/cache/update-info.json"
}

failures=0
fail() {
  echo "FAIL $1" >&2
  failures=$((failures + 1))
}

init_opt="$(make_appimage "$work/opt" "4.3.200")"
ln -s "$(basename "$init_opt")" "$work/opt/current.AppImage"

rm -f "$work/mise.log"
write_cache_update "4.3.210" corrupt
if output="$("$work/repo/scripts/update-platform.sh" beeper 2>&1)"; then
  fail "corrupt sha512 should fail: $output"
elif [[ "$output" != *"SHA-512"* || "$(readlink "$work/opt/current.AppImage")" != "Beeper-4.3.200-x86_64.AppImage" ]]; then
  fail "corrupt sha512 altered symlink or lacked SHA-512 message: $output"
else
  echo "ok   corrupt SHA-512 in updater cache is refused without touching current.AppImage"
fi

write_cache_update "4.3.210"
if ! output="$("$work/repo/scripts/update-platform.sh" --check beeper 2>&1)"; then
  fail "check mode failed: $output"
elif [[ "$output" != *"Check only: would update beeper from 4.3.200 to 4.3.210"* || "$(readlink "$work/opt/current.AppImage")" != "Beeper-4.3.200-x86_64.AppImage" || -e "$work/mise.log" ]]; then
  fail "check mode modified state: $output"
else
  echo "ok   --check reports plan without changing symlinks or calling mise"
fi

cat >"$work/server_state.json" <<'JSON'
{
  "app_version": "4.3.200",
  "spec_version": "5.0.0",
  "spec_by_version": {"4.3.210": "5.1.0"},
  "broken_versions": [],
  "api_down": false
}
JSON
rm -f "$work/mise.log"
if ! output="$("$work/repo/scripts/update-platform.sh" beeper 2>&1)"; then
  fail "compatible update from cache failed: $output"
elif [[ "$(readlink "$work/opt/current.AppImage")" != "Beeper-4.3.210-x86_64.AppImage" || "$(readlink "$work/opt/previous.AppImage")" != "Beeper-4.3.200-x86_64.AppImage" ]]; then
  fail "symlinks not updated: current=$(readlink "$work/opt/current.AppImage") prev=$(readlink "$work/opt/previous.AppImage")"
elif [[ -e "$work/cache/update-info.json" || -e "$work/cache/Beeper-4.3.210-x86_64.AppImage" ]]; then
  fail "consumed updater cache was not removed"
elif grep -q "gentalent" "$work/mise.log" || ! grep -q "daemons stop beeper" "$work/mise.log" || ! grep -q "daemons start beeper" "$work/mise.log"; then
  fail "restart did not target only beeper: $(cat "$work/mise.log")"
elif [[ "$output" != *"Review note:"* ]]; then
  fail "minor spec bump missing review note: $output"
else
  echo "ok   compatible cached update swaps symlinks, restarts only beeper, cleans cache, notes minor spec bump"
fi

cat >"$work/server_state.json" <<'JSON'
{
  "app_version": "4.3.210",
  "spec_version": "5.1.0",
  "spec_by_version": {"4.3.220": "6.0.0"},
  "broken_versions": [],
  "api_down": false
}
JSON
write_cache_update "4.3.220"
rm -f "$work/mise.log"
if output="$("$work/repo/scripts/update-platform.sh" beeper 2>&1)"; then
  fail "incompatible 6.0.0 update should have failed and rolled back: $output"
elif [[ "$(readlink "$work/opt/current.AppImage")" != "Beeper-4.3.210-x86_64.AppImage" ]]; then
  fail "failed update did not roll back current.AppImage: $(readlink "$work/opt/current.AppImage")"
elif [[ "$output" != *"Rolling back current.AppImage to Beeper-4.3.210-x86_64.AppImage"* ]]; then
  fail "rollback message missing: $output"
elif ! "$jq_bin/jq" -e '.blocked_versions | any(.version == "4.3.220")' "$work/state/platforms/beeper.json" >/dev/null; then
  fail "blocked version 4.3.220 not persisted: $(cat "$work/state/platforms/beeper.json")"
else
  echo "ok   incompatible managed update rolls back to previous version and records blocked version"
fi

rm -f "$work/mise.log"
if output="$("$work/repo/scripts/update-platform.sh" beeper 2>&1)"; then
  fail "blocked version 4.3.220 should be refused without --force: $output"
elif [[ "$output" != *"requires code changes"* || -e "$work/mise.log" ]]; then
  fail "blocked refusal wrong or touched daemon: $output"
else
  echo "ok   blocked version is refused without --force and leaves daemon untouched"
fi

cat >"$work/server_state.json" <<'JSON'
{
  "app_version": "4.3.210",
  "spec_version": "5.0.0",
  "spec_by_version": {"4.3.220": "5.0.0"},
  "broken_versions": [],
  "api_down": false
}
JSON
if ! output="$("$work/repo/scripts/update-platform.sh" --force beeper 2>&1)"; then
  fail "--force update failed once compatible: $output"
elif [[ "$(readlink "$work/opt/current.AppImage")" != "Beeper-4.3.220-x86_64.AppImage" ]]; then
  fail "--force did not install 4.3.220"
elif "$jq_bin/jq" -e '.blocked_versions | length > 0' "$work/state/platforms/beeper.json" >/dev/null; then
  fail "successful forced update did not clear block: $(cat "$work/state/platforms/beeper.json")"
else
  echo "ok   --force bypasses blocked version and clears block on compatible verify"
fi

if ! output="$("$work/repo/scripts/update-platform.sh" beeper 2>&1)"; then
  fail "download fallback failed: $output"
elif [[ "$(readlink "$work/opt/current.AppImage")" != "Beeper-4.3.230-x86_64.AppImage" ]]; then
  fail "download fallback did not swap to 4.3.230: $(readlink "$work/opt/current.AppImage")"
else
  echo "ok   download fallback resolves version from Content-Disposition and verifies update"
fi

cat >"$work/stubs/busy_bot" <<'STUB'
#!/bin/sh
if [ "$3" = "busy" ]; then
  echo "photo searches running" >&2
  exit 1
fi
exit 0
STUB
chmod +x "$work/stubs/busy_bot"
if output="$(GENTALENT_BIN="$work/stubs/busy_bot" "$work/repo/scripts/update-platform.sh" beeper 2>&1)"; then
  fail "busy bot should refuse update"
elif [[ "$output" != *"mid-operation"* ]]; then
  fail "unexpected busy error: $output"
else
  echo "ok   update refuses while live bot is mid-operation"
fi

run_opt="$work/run_opt"
mkdir -p "$run_opt"
make_appimage "$run_opt" "4.3.205" >/dev/null
make_appimage "$run_opt" "4.3.215" >/dev/null
if got="$(BEEPER_OPT_DIR="$run_opt" BEEPER_API_URL="http://127.0.0.1:1" BEEPER_CONFIG_DIR="$work/no-cfg" BEEPER_SKIP_X11_WAIT=1 BEEPER_SKIP_PGREP=1 "$work/repo/scripts/run-beeper.sh")" && [[ "$got" == "version-4.3.215" ]]; then
  echo "ok   run-beeper.sh falls back to newest versioned AppImage when current.AppImage is absent"
else
  fail "run-beeper.sh fallback got $got"
fi

ln -s "Beeper-4.3.205-x86_64.AppImage" "$run_opt/current.AppImage"
if got="$(BEEPER_OPT_DIR="$run_opt" BEEPER_API_URL="http://127.0.0.1:1" BEEPER_CONFIG_DIR="$work/no-cfg" BEEPER_SKIP_X11_WAIT=1 BEEPER_SKIP_PGREP=1 "$work/repo/scripts/run-beeper.sh")" && [[ "$got" == "version-4.3.205" ]]; then
  echo "ok   run-beeper.sh prefers current.AppImage symlink"
else
  fail "run-beeper.sh current.AppImage got $got"
fi

override_bin="$work/override.AppImage"
printf '#!/bin/sh\necho version-override\n' >"$override_bin"
chmod 755 "$override_bin"
if got="$(BEEPER_APPIMAGE="$override_bin" BEEPER_OPT_DIR="$run_opt" BEEPER_API_URL="http://127.0.0.1:1" BEEPER_CONFIG_DIR="$work/no-cfg" BEEPER_SKIP_X11_WAIT=1 BEEPER_SKIP_PGREP=1 "$work/repo/scripts/run-beeper.sh")" && [[ "$got" == "version-override" ]]; then
  echo "ok   run-beeper.sh honors BEEPER_APPIMAGE override"
else
  fail "run-beeper.sh override got $got"
fi

dup_marker="$work/dup_launched"
dup_bin="$work/dup.AppImage"
printf '#!/bin/sh\ntouch "%s"\n' "$dup_marker" >"$dup_bin"
chmod 755 "$dup_bin"
BEEPER_APPIMAGE="$dup_bin" BEEPER_API_URL="$api_url" BEEPER_CONFIG_DIR="$work/no-cfg" BEEPER_SKIP_X11_WAIT=1 BEEPER_SKIP_PGREP=1 "$work/repo/scripts/run-beeper.sh" >/dev/null 2>"$work/run_ext.err" &
ext_pid=$!
for _ in $(seq 1 30); do
  grep -q "already running externally" "$work/run_ext.err" 2>/dev/null && break
  sleep 0.1
done
kill -TERM "$ext_pid" >/dev/null 2>&1 || true
wait "$ext_pid" >/dev/null 2>&1 || true
if [[ -e "$dup_marker" ]] || ! grep -q "already running externally" "$work/run_ext.err"; then
  fail "run-beeper.sh launched duplicate or did not detect external Beeper: $(cat "$work/run_ext.err")"
else
  echo "ok   run-beeper.sh attaches without launching duplicate when Beeper is already running"
fi

exit "$failures"

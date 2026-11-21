#!/usr/bin/env bash
set -euo pipefail

unset ELECTRON_RUN_AS_NODE
export DISPLAY="${DISPLAY:-:0}"
export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-0}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=/run/user/$(id -u)/bus}"
export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$PATH"

opt_dir="${BEEPER_OPT_DIR:-$HOME/.local/opt/beeper}"
api_url="${BEEPER_API_URL:-http://127.0.0.1:23373}"
config_dir="${BEEPER_CONFIG_DIR:-$HOME/.config/BeeperTexts}"
x11_socket="${BEEPER_X11_SOCKET:-/tmp/.X11-unix/X0}"
proc_name="${BEEPER_PROCESS_NAME:-beepertexts}"

is_beeper_running() {
  if [[ -z "${BEEPER_SKIP_PGREP:-}" ]] && pgrep -x "$proc_name" >/dev/null 2>&1; then
    return 0
  fi
  if command -v curl >/dev/null 2>&1 && curl -sf "${api_url}/v1/info" >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

resolve_beeper_target() {
  if [[ -n "${BEEPER_APPIMAGE:-}" ]]; then
    printf '%s\n' "$BEEPER_APPIMAGE"
    return 0
  fi
  if [[ -n "${BEEPER_BIN:-}" ]]; then
    printf '%s\n' "$BEEPER_BIN"
    return 0
  fi
  if [[ -e "$opt_dir/current.AppImage" ]]; then
    printf '%s\n' "$opt_dir/current.AppImage"
    return 0
  fi
  if [[ -d "$opt_dir" ]]; then
    local latest
    latest="$(find "$opt_dir" -maxdepth 1 \( -type f -o -type l \) -name 'Beeper-*.AppImage' -printf '%f\t%p\n' 2>/dev/null | sort -V | tail -n 1 | cut -f2-)"
    if [[ -n "$latest" ]]; then
      printf '%s\n' "$latest"
      return 0
    fi
  fi
  return 1
}

if is_beeper_running; then
  echo "Beeper is already running externally; monitoring existing process without launching a duplicate." >&2
  trap 'exit 0' INT TERM
  while is_beeper_running; do
    sleep 5 &
    wait $! || true
  done
  exit 0
fi

if [[ -z "${BEEPER_SKIP_X11_WAIT:-}" ]]; then
  for _ in $(seq 1 30); do
    [[ -S "$x11_socket" ]] && break
    sleep 1
  done
fi

if [[ -z "${BEEPER_SKIP_PGREP:-}" ]] && ! pgrep -x "$proc_name" >/dev/null 2>&1; then
  rm -f "$config_dir/SingletonLock" \
        "$config_dir/SingletonCookie" \
        "$config_dir/SingletonSocket"
fi

if [[ -f "$config_dir/window.json" ]] && command -v python3 >/dev/null 2>&1; then
  BEEPER_CONFIG_DIR="$config_dir" python3 -c 'import json, os, pathlib
p = pathlib.Path(os.environ["BEEPER_CONFIG_DIR"]) / "window.json"
if p.exists():
    d = json.loads(p.read_text())
    m = d.setdefault("windowState", {}).setdefault("main", {})
    m["isVisible"] = True
    m["isMinimized"] = False
    p.write_text(json.dumps(d))
'
fi

target="$(resolve_beeper_target || true)"
if [[ -z "$target" || ! -x "$target" ]]; then
  echo "No executable Beeper AppImage found (checked BEEPER_APPIMAGE, BEEPER_BIN, $opt_dir/current.AppImage, and $opt_dir/Beeper-*.AppImage)." >&2
  exit 1
fi

exec "$target"

#!/usr/bin/env bash
set -euo pipefail

unset ELECTRON_RUN_AS_NODE
export DISPLAY="${DISPLAY:-:0}"
export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-0}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=/run/user/$(id -u)/bus}"
export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$PATH"

for _ in $(seq 1 30); do
  [[ -S /tmp/.X11-unix/X0 ]] && break
  sleep 1
done

if ! pgrep -x beepertexts >/dev/null 2>&1; then
  rm -f "$HOME/.config/BeeperTexts/SingletonLock" \
        "$HOME/.config/BeeperTexts/SingletonCookie" \
        "$HOME/.config/BeeperTexts/SingletonSocket"
fi

python3 -c 'import json, pathlib
p = pathlib.Path("~/.config/BeeperTexts/window.json").expanduser()
if p.exists():
    d = json.loads(p.read_text())
    m = d.setdefault("windowState", {}).setdefault("main", {})
    m["isVisible"] = True
    m["isMinimized"] = False
    p.write_text(json.dumps(d))
'

exec "$HOME/.local/opt/beeper/Beeper-4.3.308-x86_64.AppImage"

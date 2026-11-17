#!/usr/bin/env bash
set -euo pipefail

unset ELECTRON_RUN_AS_NODE
export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$PATH"
export PYTHONUNBUFFERED=1

cd "$HOME/ai/gentalent"

for _ in $(seq 1 30); do
  if python3 -m gentalent doctor --chat >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

needs_arm="$(python3 -c '
import sqlite3, pathlib, json
cfg = json.loads(pathlib.Path("config.json").read_text())
db_path = pathlib.Path(cfg["database"]).expanduser()
if not db_path.is_absolute():
    db_path = pathlib.Path.cwd() / db_path
if not db_path.exists():
    print("yes")
else:
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    try:
        row = db.execute("SELECT value FROM meta WHERE key='\''armed'\''").fetchone()
        print("no" if row and row[0] else "yes")
    except sqlite3.OperationalError:
        print("yes")
    finally:
        db.close()
')"

if [[ "$needs_arm" == "yes" ]]; then
  python3 -m gentalent arm --live
fi

exec python3 -u -m gentalent run --live

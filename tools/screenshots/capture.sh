#!/usr/bin/env bash
# Regenerates docs/images/*.png from the REAL service using clearly fake DEMO data
# (no real backups, nothing leaves your machine). Needs: Go, Python 3, Node + Playwright
# (npm i -g playwright && npx playwright install chromium). Optional: Pillow for the hero image.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/docs/images"; mkdir -p "$OUT"
W="$(mktemp -d)"; trap 'kill $(jobs -p) 2>/dev/null || true; rm -rf "$W"' EXIT
PORT=5892; FAKE=5894
export NODE_PATH="${NODE_PATH:-$(npm root -g 2>/dev/null || true)}"

(cd "$ROOT" && go build -o "$W/syncamp" ./cmd/syncamp)
mkdir -p "$W/data" "$W/profiles" "$W/bin"; chmod 755 "$W" "$W/bin"
printf '#!/bin/sh\necho "Backup completed in 1 minute"\n' > "$W/bin/client.sh"; chmod 755 "$W/bin/client.sh"
for n in "Documents:127" "Photos:127" "Work Laptop:62" "Scratch:0"; do
  echo "<p><schedulerDays>${n#*:}</schedulerDays></p>" > "$W/profiles/${n%%:*}.syncrify"
done
python3 -I - "$W" "$PORT" "$FAKE" <<'PY'
import json, sys
w, port, fake = sys.argv[1], sys.argv[2], sys.argv[3]
json.dump({"listenAddr": f"127.0.0.1:{port}", "clientPath": f"{w}/bin/client.sh", "profileDirs": [f"{w}/profiles"],
           "server": {"url": f"http://127.0.0.1:{fake}", "checkInterval": "1m"}, "mode": "monitor",
           "profiles": {"Scratch": {"enabled": False}}}, open(f"{w}/data/config.json", "w"))
PY
history() { # $1 = attention | healthy
python3 -I - "$W" "$1" <<'PY'
import json, sys, datetime
w, state = sys.argv[1], sys.argv[2]
now = datetime.datetime.now(datetime.timezone.utc)
def run(p, status, hours, err=""):
    t = (now - datetime.timedelta(hours=hours)).isoformat().replace("+00:00", "Z")
    r = {"profile": p, "start": t, "end": t, "status": status, "exitCode": 0, "trigger": "observed"}
    if err: r["error"] = err
    return r
if state == "attention":
    h = [run("Photos", "success", 50), run("Photos", "failed", 2, "Unable to connect to the server backup.lan:5800"),
         run("Documents", "success", 3),
         run("Work Laptop", "warning", 1, "Backup started on Mon and completed with one or more error.")]
else:
    h = [run("Documents", "success", 3), run("Photos", "success", 5), run("Work Laptop", "success", 1)]
json.dump(h, open(f"{w}/data/history.json", "w"))
PY
}
start() { "$W/syncamp" -config "$W/data/config.json" run >/dev/null 2>&1 & echo $! > "$W/pid"; sleep 3; }
stop()  { kill "$(cat "$W/pid")" 2>/dev/null || true; sleep 1; }

python3 -I "$ROOT/tools/screenshots/fake_syncrify.py" "$FAKE" & sleep 1
history attention; start
TOKEN="$("$W/syncamp" -config "$W/data/config.json" token)"
node "$ROOT/tools/screenshots/capture.js" "$PORT" "$TOKEN" attention "$OUT/popover-attention.png"
node "$ROOT/tools/screenshots/capture.js" "$PORT" "$TOKEN" signin    "$OUT/signin.png"
stop; history healthy; start
node "$ROOT/tools/screenshots/capture.js" "$PORT" "$TOKEN" healthy   "$OUT/popover-healthy.png"
stop

python3 -I - "$OUT" <<'PY' || echo "(Pillow not installed: skipped hero.png)"
import sys
from PIL import Image
o = sys.argv[1]
a, b = Image.open(f"{o}/popover-attention.png"), Image.open(f"{o}/popover-healthy.png")
gap, pad = 56, 56
h = max(a.height, b.height) + 2 * pad
c = Image.new("RGB", (a.width + b.width + gap + 2 * pad, h), (238, 240, 250))
c.paste(a, (pad, pad)); c.paste(b, (pad + a.width + gap, pad))
c.save(f"{o}/hero.png", optimize=True)  # lossless: palette reduction turns the green status dots teal
for n in ("popover-attention", "popover-healthy", "signin"):
    Image.open(f"{o}/{n}.png").save(f"{o}/{n}.png", optimize=True)
PY
echo "wrote $OUT"; ls -la "$OUT"

#!/usr/bin/env bash
# One-shot BYOK check: run the harness on THIS machine, drive a turn from prod.
#
# Run it from a normal terminal, NOT from inside a Claude Code session —
# claude-code-acp reads your credentials from the macOS Keychain, and a
# sandboxed parent cannot reach them.
#
#   ./test-byok.sh
#
# It builds the runner, connects it to prod, opens a session bound to this
# device, sends one prompt, prints what your Claude answered, and cleans up.
set -euo pipefail

HOST="${HOST:-root@89.167.94.146}"
SERVER="${SERVER:-https://agent.freehire.dev}"
PROMPT="${PROMPT:-Reply with exactly three words.}"

cd "$(dirname "$0")"
say() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

say "0/4  checking prerequisites"
command -v claude-code-acp >/dev/null || {
  echo "claude-code-acp is not on PATH — install it first: npm i -g @zed-industries/claude-code-acp"
  exit 1
}
if [ -n "${CLAUDECODE:-}" ]; then
  echo "warning: CLAUDECODE is set, so you are inside a Claude Code session."
  echo "The runner strips it for the harness, but the Keychain may still be out of reach."
  echo "If the turn fails with 'Authentication required', rerun from a plain terminal."
fi

say "1/4  building the runner"
go build -o /tmp/freehire-runner ./cmd/freehire
echo "ok"

say "2/4  connecting this machine"
LOG=$(mktemp)
/tmp/freehire-runner runner --server "$SERVER" >"$LOG" 2>&1 &
RUNNER_PID=$!
cleanup() {
  kill "$RUNNER_PID" 2>/dev/null || true
  [ -n "${SESSION:-}" ] && ssh -o ConnectTimeout=10 "$HOST" "sudo -u freehire python3 -c \"
import socket, json
s = socket.socket(socket.AF_UNIX); s.connect('/run/freehire-agent/daemon.sock')
f = s.makefile('rwb')
f.write((json.dumps({'op': 'close', 'session': '$SESSION'}) + '\n').encode()); f.flush()
f.readline()
\"" >/dev/null 2>&1 || true
  rm -f "$LOG"
}
trap cleanup EXIT

for _ in $(seq 1 30); do
  grep -q "connected to" "$LOG" && break
  sleep 1
done
grep -q "connected to" "$LOG" || { echo "runner never connected:"; cat "$LOG"; exit 1; }
DEVICE=$(sed -n 's/^device \([a-f0-9]*\) .*/\1/p' "$LOG" | head -1)
echo "connected as device $DEVICE"

say "3/4  opening a session on this device"
KEY=$(python3 -c "import json;print(json.load(open('$HOME/.freehire/creds.json'))['token'])")
JWT=$(curl -sS -X POST "$SERVER/auth/runner-token" -H "Authorization: Bearer $KEY" |
  python3 -c "import json,sys;print(json.load(sys.stdin)['token'])")
RESP=$(curl -sS -X POST "$SERVER/sessions" \
  -H 'content-type: application/json' -H "Cookie: hire_token=$JWT" \
  -d "{\"harness\":\"claude\",\"device_id\":\"$DEVICE\"}")
SESSION=$(printf '%s' "$RESP" | sed -n 's/.*"session_id":"\([^"]*\)".*/\1/p')
[ -n "$SESSION" ] || { echo "session was refused: $RESP"; echo "--- runner log ---"; cat "$LOG"; exit 1; }
echo "session $SESSION — your local claude-code-acp is now running it"

say "4/4  asking it something"
ssh -o ConnectTimeout=20 "$HOST" "sudo -u freehire python3 - <<PY
import socket, json, time
s = socket.socket(socket.AF_UNIX); s.connect('/run/freehire-agent/daemon.sock')
f = s.makefile('rwb')
def send(o):
    f.write((json.dumps(o) + '\n').encode()); f.flush()
sid = '$SESSION'
send({'op': 'attach', 'session': sid})
send({'op': 'acquire_input', 'session': sid})
send({'op': 'send', 'session': sid, 'text': '''$PROMPT'''})
deadline = time.time() + 180
while time.time() < deadline:
    line = f.readline()
    if not line:
        break
    ev = json.loads(line)
    if ev.get('kind') == 'frame':
        e = ev['entry']['event']
        if e.get('type') == 'assistant_text':
            print('CLAUDE:', e.get('text', '').strip()[:300])
        elif e.get('type') == 'result':
            print('turn finished:', e.get('stop_reason'))
            break
    elif ev.get('kind') == 'error':
        print('ERROR:', ev.get('code'), str(ev.get('message'))[:200])
        break
PY"

echo
echo "If Claude answered above, BYOK works: the model ran here, the session lived on the server."
echo "If it said 'Authentication required', log in with \`claude\` first, or rerun outside Claude Code."

#!/usr/bin/env bash
# Start the runner and keep it up, so the assistant's sessions run on THIS
# machine with your own Claude credentials.
#
# Run it from a normal terminal, NOT from inside a Claude Code session:
# claude-code-acp reads your credentials from the macOS Keychain, and a
# sandboxed parent cannot reach them.
#
#   ./run-byok.sh
#
# Then just use https://agent.freehire.dev as usual — while this is running,
# your sessions are routed here automatically. Ctrl-C to stop and go back to
# server-hosted sessions.
set -euo pipefail

HOST="${HOST:-root@89.167.94.146}"
SERVER="${SERVER:-https://agent.freehire.dev}"
USER_ID="${USER_ID:-9700a722-3d78-49dc-a767-05981799a379}"
# The token is short-lived; refresh it by restarting this script.
TTL="${TTL:-28800}"

cd "$(dirname "$0")"

command -v claude-code-acp >/dev/null || {
  echo "claude-code-acp is not on PATH."
  echo "Install it with: npm i -g @zed-industries/claude-code-acp"
  exit 1
}
if [ -n "${CLAUDECODE:-}" ]; then
  echo "You are inside a Claude Code session. The harness will start, but it may"
  echo "not reach your Keychain credentials — if turns fail with 'Authentication"
  echo "required', rerun this from a plain terminal."
  echo
fi

echo "building..."
go build -o ./freehire ./cmd/freehire

# Temporary: freehire will hand runners a token directly once it exchanges
# fhk_ keys for one. Until then we sign it on the server.
echo "getting a token..."
JWT=$(ssh -o ConnectTimeout=10 "$HOST" "S=\$(grep '^ROY_JWT_SECRET=' /opt/freehire/env/agent.env | cut -d= -f2-)
python3 -c \"
import jwt, time
n = int(time.time())
print(jwt.encode({'sub': '$USER_ID', 'iat': n, 'exp': n + $TTL}, '\$S', algorithm='HS256'))
\"")
[ -n "$JWT" ] || { echo "could not get a token"; exit 1; }

echo
echo "Starting. While this runs, sessions you open at $SERVER"
echo "run here, on your Claude. Ctrl-C to stop."
echo
exec env ROY_RUNNER_TOKEN="$JWT" ./freehire runner --server "$SERVER"

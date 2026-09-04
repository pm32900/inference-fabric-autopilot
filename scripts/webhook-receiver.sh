#!/usr/bin/env bash
#
# webhook-receiver.sh — listen for IFA alert webhooks and pretty-print them.
#
# Usage: ./scripts/webhook-receiver.sh [PORT]
#   PORT defaults to 9999.
#
# Start this before enabling alerting so you can verify that alerts fire once,
# do not repeat on the following cycle (suppressed), and resolve when the
# condition clears.
#
# Point IFA at it:
#   export IFA_ALERTING_WEBHOOK_URL=http://HOST:9999/hook
#   # or in config.yaml:
#   alerting:
#     enabled: true
#     webhook_url: http://HOST:9999/hook

set -euo pipefail

PORT="${1:-9999}"

# Write the handler to a temp file so shellcheck can validate this script
# without also trying to parse Python.
SCRIPT="$(mktemp /tmp/ifa-receiver-XXXXXX.py)"
trap 'rm -f "${SCRIPT}"' EXIT

cat > "${SCRIPT}" <<'PYEOF'
import sys
import json
import datetime
import http.server

PORT = int(sys.argv[1])

RED    = "\033[31m"
YELLOW = "\033[33m"
GREEN  = "\033[32m"
BOLD   = "\033[1m"
RESET  = "\033[0m"

EVENT_COLOR = {
    "firing":    RED,
    "escalated": YELLOW,
    "resolved":  GREEN,
}


def ts():
    return datetime.datetime.now().strftime("%H:%M:%S")


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):  # noqa: N802
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        self.send_response(204)
        self.end_headers()
        try:
            payload = json.loads(body)
        except json.JSONDecodeError as exc:
            print(f"[{ts()}] bad JSON from IFA: {exc}")
            return

        event   = payload.get("event", "unknown")
        cluster = payload.get("cluster", "-")
        color   = EVENT_COLOR.get(event, BOLD)

        print(f"[{ts()}]  {color}{event.upper():10}{RESET}  cluster={cluster}")
        for f in payload.get("findings", []):
            sev  = f.get("severity", "?")
            code = f.get("code", "?")
            wl   = f.get("workload_name", "?")
            ns   = f.get("namespace", "?")
            title = f.get("title", "")
            print(f"           {sev:8}  {code}  {ns}/{wl}  {title}")
        print()

    def log_message(self, *_):
        pass  # silence the default per-request access log


print(f"Listening on http://0.0.0.0:{PORT}  (Ctrl-C to stop)")
print()
with http.server.HTTPServer(("0.0.0.0", PORT), Handler) as srv:
    srv.serve_forever()
PYEOF

exec python3 "${SCRIPT}" "${PORT}"

#!/usr/bin/env bash
#
# demo.sh — run the control plane against a simulated inference fleet and print
# what it diagnoses.
#
# The simulation serves Prometheus exposition in vLLM's own format over HTTP.
# Everything downstream of that socket is the code that runs in production: the
# exposition parser, the histogram quantile estimation, the counter-to-rate
# conversion, and the rule engine. Nothing here prints a canned result.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${ROOT}/bin"
PORT="${IFA_DEMO_PORT:-8080}"
# The demo shortens the rule engine's sustain window to 5 seconds, so findings
# appear after roughly this long rather than after the 45-second production
# default. Nothing else about the rules is relaxed.
SETTLE="${IFA_DEMO_SETTLE:-12}"

if [[ ! -x "${BIN}/control-plane" || ! -x "${BIN}/ifa" ]]; then
  echo "==> Building"
  make -C "${ROOT}" build
fi

cleanup() {
  if [[ -n "${CP_PID:-}" ]] && kill -0 "${CP_PID}" 2>/dev/null; then
    kill "${CP_PID}" 2>/dev/null || true
    wait "${CP_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

LOG="$(mktemp -t ifa-demo.XXXXXX)"
echo "==> Starting the control plane against 7 simulated workloads (logs: ${LOG})"
IFA_ADDRESS=":${PORT}" IFA_LOG_FORMAT=text "${BIN}/control-plane" --demo >"${LOG}" 2>&1 &
CP_PID=$!

export IFA_URL="http://localhost:${PORT}"

# Wait for the API rather than sleeping a fixed amount: readiness flips once the
# first collection cycle completes.
for _ in $(seq 1 50); do
  if curl -fsS "${IFA_URL}/api/v1/readyz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "${CP_PID}" 2>/dev/null; then
    echo "control plane exited during startup:" >&2
    cat "${LOG}" >&2
    exit 1
  fi
  sleep 0.2
done

echo "==> Letting the rule engine accumulate its sustain window (${SETTLE}s)"
sleep "${SETTLE}"

echo
echo "════════════════════════════════════════════════════════════════════════"
echo " Telemetry"
echo "════════════════════════════════════════════════════════════════════════"
"${BIN}/ifa" telemetry

echo
echo "════════════════════════════════════════════════════════════════════════"
echo " Findings"
echo "════════════════════════════════════════════════════════════════════════"
"${BIN}/ifa" recommendations

cat <<EOF

════════════════════════════════════════════════════════════════════════
 The control plane is still running on ${IFA_URL}. Try:

   curl -s ${IFA_URL}/api/v1/recommendations | jq '.items[0]'
   curl -s ${IFA_URL}/api/v1/rules | jq '.items[].code'
   curl -s ${IFA_URL}/metrics | head -20
   bin/ifa recommendations -severity critical
   bin/ifa telemetry -workload chat-llama3-8b

 Press Ctrl-C to stop.
════════════════════════════════════════════════════════════════════════
EOF

wait "${CP_PID}"

#!/usr/bin/env bash
# Inject API errors (and optional latency) into running instances.
#
# Usage: ADMIN_TOKEN=... ./inject.sh <base-url> <error-rate 0..1> <seconds> [extra-latency-ms]
#   e.g. ADMIN_TOKEN=$TOKEN ./inject.sh https://moimroom.example.com 0.3 120
#
# Faults live in each process. Behind a load balancer one request reaches one
# instance, so the call is repeated REPEAT times (default 10) to reach every
# replica; the output lists the instances that took it (X-Served-By).
set -euo pipefail

if [ $# -lt 3 ]; then
  sed -n '2,9p' "$0"
  exit 2
fi
: "${ADMIN_TOKEN:?ADMIN_TOKEN is required}"
base=${1%/}
rate=$2
seconds=$3
latency=${4:-0}
body=$(printf '{"errorRate":%s,"extraLatencyMs":%s,"durationSeconds":%s}' "$rate" "$latency" "$seconds")

for _ in $(seq "${REPEAT:-10}"); do
  curl -sS -o /dev/null -D - -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
    -d "$body" "$base/admin/fault" |
    awk 'NR==1 {status=$2} tolower($1)=="x-served-by:" {print status, $2}' | tr -d '\r'
done | sort | uniq -c

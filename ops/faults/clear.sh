#!/usr/bin/env bash
# Clear injected faults on running instances.
#
# Usage: ADMIN_TOKEN=... ./clear.sh <base-url>
#
# Repeats REPEAT times (default 10) to reach every replica behind a load
# balancer and lists the instances that answered.
set -euo pipefail

if [ $# -lt 1 ]; then
  sed -n '2,7p' "$0"
  exit 2
fi
: "${ADMIN_TOKEN:?ADMIN_TOKEN is required}"
base=${1%/}

for _ in $(seq "${REPEAT:-10}"); do
  curl -sS -o /dev/null -D - -X DELETE \
    -H "Authorization: Bearer $ADMIN_TOKEN" "$base/admin/fault" |
    awk 'NR==1 {status=$2} tolower($1)=="x-served-by:" {print status, $2}' | tr -d '\r'
done | sort | uniq -c

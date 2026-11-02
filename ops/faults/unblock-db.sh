#!/usr/bin/env bash
# Remove the NetworkPolicy created by block-db.sh.
#
# Usage: ./unblock-db.sh <kube-context> <namespace>
set -euo pipefail

if [ $# -lt 2 ]; then
  sed -n '2,4p' "$0"
  exit 2
fi

kubectl --context "$1" -n "$2" delete networkpolicy moimroom-block-db --ignore-not-found

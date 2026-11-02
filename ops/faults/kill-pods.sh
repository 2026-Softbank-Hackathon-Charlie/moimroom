#!/usr/bin/env bash
# Kill the app pods at once, without a graceful shutdown, as a crash would.
#
# Usage: ./kill-pods.sh <kube-context> <namespace> [label-selector]
#   default selector: app.kubernetes.io/name=moimroom
set -euo pipefail

if [ $# -lt 2 ]; then
  sed -n '2,5p' "$0"
  exit 2
fi
ctx=$1
ns=$2
selector=${3:-app.kubernetes.io/name=moimroom}

kubectl --context "$ctx" -n "$ns" get pods -l "$selector" -o wide
kubectl --context "$ctx" -n "$ns" delete pods -l "$selector" --grace-period=0 --force --wait=false

#!/usr/bin/env bash
# Cut the app off from PostgreSQL with a NetworkPolicy that allows all egress
# except the database port (default 5432, override with DB_PORT).
#
# Usage: ./block-db.sh <kube-context> <namespace>
#
# Needs a CNI that enforces NetworkPolicy with endPort (Calico, Cilium, GKE
# Dataplane V2, EKS with network policy enabled). Established connections may
# survive; the app recycles pooled connections within about 35 s, so the block
# is complete by then. Undo with ./unblock-db.sh.
set -euo pipefail

if [ $# -lt 2 ]; then
  sed -n '2,11p' "$0"
  exit 2
fi
ctx=$1
ns=$2
port=${DB_PORT:-5432}

kubectl --context "$ctx" -n "$ns" apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: moimroom-block-db
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: moimroom
  policyTypes: ["Egress"]
  egress:
    - ports:
        - protocol: TCP
          port: 1
          endPort: $((port - 1))
        - protocol: TCP
          port: $((port + 1))
          endPort: 65535
        - protocol: UDP
          port: 1
          endPort: 65535
EOF

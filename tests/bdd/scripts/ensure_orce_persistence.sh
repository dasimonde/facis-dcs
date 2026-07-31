#!/usr/bin/env bash
set -euo pipefail

: "${KUBECTL_BIN:?KUBECTL_BIN is required}"
: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${HELM_RELEASE:?HELM_RELEASE is required}"

PV_NAME="${K8S_NAMESPACE}-${HELM_RELEASE}-orce"
PVC_NAME="${HELM_RELEASE}-orce"
PV_PHASE="$("${KUBECTL_BIN}" get pv "${PV_NAME}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"

# A retained BDD volume cannot bind to a newly created Helm PVC after an
# uninstall. Recreate only that released/failed test volume; a Bound volume is
# deliberately kept so ORCE data survives pod restarts.
if [[ "${PV_PHASE}" == "Released" || "${PV_PHASE}" == "Failed" ]]; then
  "${KUBECTL_BIN}" delete pv "${PV_NAME}"
fi

cat <<EOF | "${KUBECTL_BIN}" apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ${PV_NAME}
spec:
  capacity:
    storage: 2Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: ""
  claimRef:
    namespace: ${K8S_NAMESPACE}
    name: ${PVC_NAME}
  hostPath:
    path: /var/local/dcs-bdd/${PV_NAME}
    type: DirectoryOrCreate
EOF

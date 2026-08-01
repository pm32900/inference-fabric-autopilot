#!/usr/bin/env bash
# load-airgap-bundle.sh
#
# Extracts an IFA air-gap bundle, verifies checksums, and loads container
# images into Docker or containerd. Prints the Helm install command to run
# after image loading is complete.
#
# Run this on the air-gapped machine after transferring airgap-bundle.tar.gz.
#
# Usage:
#   ./load-airgap-bundle.sh [BUNDLE_TARBALL] [--runtime docker|containerd] [--namespace NAMESPACE]
#
# Arguments:
#   BUNDLE_TARBALL   Path to the bundle tarball (default: airgap-bundle.tar.gz)
#   --runtime        Container runtime to load images into: docker or containerd (default: docker)
#   --namespace      Kubernetes namespace to deploy into (default: inference)
#
# Requirements:
#   - docker (if --runtime docker) OR ctr (if --runtime containerd)
#   - sha256sum or shasum
#   - tar

set -euo pipefail

# --- Defaults ---
BUNDLE_TARBALL="airgap-bundle.tar.gz"
RUNTIME="docker"
NAMESPACE="inference"
BUNDLE_DIR="airgap-bundle"

# --- Argument parsing ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --runtime)
      RUNTIME="$2"
      shift 2
      ;;
    --namespace)
      NAMESPACE="$2"
      shift 2
      ;;
    --*)
      echo "ERROR: Unknown option: $1"
      echo "Usage: $0 [BUNDLE_TARBALL] [--runtime docker|containerd] [--namespace NAMESPACE]"
      exit 1
      ;;
    *)
      BUNDLE_TARBALL="$1"
      shift
      ;;
  esac
done

echo "==> Inference Fabric Autopilot — Air-Gap Bundle Load"
echo "    Bundle  : ${BUNDLE_TARBALL}"
echo "    Runtime : ${RUNTIME}"
echo "    Namespace: ${NAMESPACE}"
echo ""

# --- Validate runtime flag ---
if [[ "${RUNTIME}" != "docker" && "${RUNTIME}" != "containerd" ]]; then
  echo "ERROR: --runtime must be 'docker' or 'containerd', got: ${RUNTIME}"
  exit 1
fi

# --- Prerequisite checks ---
echo "--> Checking prerequisites..."

if ! command -v tar &>/dev/null; then
  echo "ERROR: tar not found."
  exit 1
fi

if [[ "${RUNTIME}" == "docker" ]] && ! command -v docker &>/dev/null; then
  echo "ERROR: docker not found. Install Docker or use --runtime containerd."
  exit 1
fi

if [[ "${RUNTIME}" == "containerd" ]] && ! command -v ctr &>/dev/null; then
  echo "ERROR: ctr not found. Install containerd tooling or use --runtime docker."
  exit 1
fi

# Prefer sha256sum (Linux); fall back to shasum -a 256 (macOS)
if command -v sha256sum &>/dev/null; then
  SHA_CMD="sha256sum"
elif command -v shasum &>/dev/null; then
  SHA_CMD="shasum -a 256"
else
  echo "ERROR: neither sha256sum nor shasum found."
  exit 1
fi

echo "    sha command: ${SHA_CMD}"
echo ""

# --- Check bundle tarball exists ---
if [[ ! -f "${BUNDLE_TARBALL}" ]]; then
  echo "ERROR: Bundle tarball not found: ${BUNDLE_TARBALL}"
  echo "       Transfer airgap-bundle.tar.gz to this machine before running."
  exit 1
fi

# --- Extract bundle ---
echo "--> Extracting ${BUNDLE_TARBALL}..."
tar -xzf "${BUNDLE_TARBALL}"

if [[ ! -d "${BUNDLE_DIR}" ]]; then
  echo "ERROR: Expected directory '${BUNDLE_DIR}' after extraction — not found."
  echo "       The bundle tarball may be corrupt or was created with a different BUNDLE_DIR name."
  exit 1
fi

cd "${BUNDLE_DIR}"
echo "    Extracted to: $(pwd)"
echo ""

# --- Verify checksums ---
echo "--> Verifying checksums..."
if [[ ! -f "checksums.sha256" ]]; then
  echo "ERROR: checksums.sha256 not found in bundle. Cannot verify integrity."
  exit 1
fi

if ! ${SHA_CMD} -c checksums.sha256; then
  echo ""
  echo "ERROR: Checksum verification FAILED. The bundle may be corrupt or tampered with."
  echo "       Do not load images from a bundle that fails checksum verification."
  exit 1
fi

echo ""
echo "    Checksum verification PASSED."
echo ""

# --- Load images ---
echo "--> Loading container images (runtime: ${RUNTIME})..."

load_image_docker() {
  local tarfile="$1"
  echo "    docker load -i ${tarfile}"
  docker load -i "${tarfile}"
}

load_image_containerd() {
  local tarfile="$1"
  echo "    ctr -n k8s.io images import ${tarfile}"
  ctr -n k8s.io images import "${tarfile}"
}

for tarfile in *.tar; do
  if [[ ! -f "${tarfile}" ]]; then
    echo "    WARNING: No .tar files found in bundle directory."
    break
  fi
  echo "--> Loading ${tarfile}..."
  if [[ "${RUNTIME}" == "docker" ]]; then
    load_image_docker "${tarfile}"
  else
    load_image_containerd "${tarfile}"
  fi
  echo ""
done

# --- Find Helm chart ---
CHART_TGZ=$(ls *.tgz 2>/dev/null | head -1)
if [[ -z "${CHART_TGZ}" ]]; then
  echo "WARNING: No Helm chart .tgz found in bundle. Helm install step will need to be done manually."
  CHART_TGZ="autopilot-*.tgz"
fi

# --- Print Helm install command ---
echo ""
echo "==> Images loaded successfully."
echo ""
echo "==> Next steps:"
echo ""
echo "    1. Create the namespace (if it does not exist):"
echo "       kubectl create namespace ${NAMESPACE}"
echo ""
echo "    2. Install with Helm:"
echo "       helm install autopilot ./${CHART_TGZ} \\"
echo "         --namespace ${NAMESPACE} \\"
echo "         --values values-airgapped.yaml \\"
echo "         --wait --timeout 120s"
echo ""
echo "    3. Verify the rollout:"
echo "       kubectl get pods -n ${NAMESPACE}"
echo ""
echo "    4. Check health:"
echo "       kubectl port-forward -n ${NAMESPACE} svc/autopilot 8080:8080 &"
echo "       curl http://localhost:8080/healthz"
echo ""
echo "    See docs/AIRGAPPED_DEPLOYMENT.md for the full procedure."

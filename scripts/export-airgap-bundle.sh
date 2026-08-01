#!/usr/bin/env bash
# export-airgap-bundle.sh
#
# Builds IFA container images, packages the Helm chart and docs, generates
# checksums, and produces airgap-bundle.tar.gz in the repo root.
#
# Run this on an internet-connected machine before transferring the bundle
# to a restricted environment.
#
# Usage:
#   ./scripts/export-airgap-bundle.sh [VERSION]
#
# Arguments:
#   VERSION   Image tag to use (default: dev)
#
# Requirements:
#   - Docker (to build and save images)
#   - Helm 3 (to package the chart)
#   - sha256sum or shasum (for checksums)

set -euo pipefail

# --- Configuration ---
VERSION="${1:-dev}"
CONTROL_PLANE_IMAGE="ifa-control-plane:${VERSION}"
NODE_AGENT_IMAGE="ifa-node-agent:${VERSION}"
CHART_DIR="deploy/helm/autopilot"
BUNDLE_DIR="airgap-bundle"
OUTPUT_TARBALL="airgap-bundle.tar.gz"

# Resolve repo root (script may be called from any directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

echo "==> Inference Fabric Autopilot — Air-Gap Bundle Export"
echo "    Version : ${VERSION}"
echo "    Repo    : ${REPO_ROOT}"
echo "    Output  : ${REPO_ROOT}/${OUTPUT_TARBALL}"
echo ""

# --- Prerequisite checks ---
echo "--> Checking prerequisites..."

if ! command -v docker &>/dev/null; then
  echo "ERROR: docker not found. Install Docker before running this script."
  exit 1
fi

if ! command -v helm &>/dev/null; then
  echo "ERROR: helm not found. Install Helm 3 before running this script."
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

# --- Clean previous bundle ---
echo "--> Cleaning previous bundle artifacts..."
rm -rf "${BUNDLE_DIR}"
rm -f "${OUTPUT_TARBALL}"
mkdir -p "${BUNDLE_DIR}"

# --- Build container images ---
echo "--> Building control-plane image: ${CONTROL_PLANE_IMAGE}"
docker build \
  -f Dockerfile.control-plane \
  -t "${CONTROL_PLANE_IMAGE}" \
  .

echo "--> Building node-agent image: ${NODE_AGENT_IMAGE}"
docker build \
  -f Dockerfile.node-agent \
  -t "${NODE_AGENT_IMAGE}" \
  .

# --- Save images as tarballs ---
echo "--> Saving images to tarballs..."
docker save "${CONTROL_PLANE_IMAGE}" -o "${BUNDLE_DIR}/ifa-control-plane.tar"
echo "    Saved: ${BUNDLE_DIR}/ifa-control-plane.tar ($(du -sh "${BUNDLE_DIR}/ifa-control-plane.tar" | cut -f1))"

docker save "${NODE_AGENT_IMAGE}" -o "${BUNDLE_DIR}/ifa-node-agent.tar"
echo "    Saved: ${BUNDLE_DIR}/ifa-node-agent.tar ($(du -sh "${BUNDLE_DIR}/ifa-node-agent.tar" | cut -f1))"

# --- Package Helm chart ---
echo "--> Packaging Helm chart..."
helm package "${CHART_DIR}" --destination "${BUNDLE_DIR}"
CHART_TGZ=$(ls "${BUNDLE_DIR}"/*.tgz 2>/dev/null | head -1)
if [ -z "${CHART_TGZ}" ]; then
  echo "ERROR: helm package did not produce a .tgz file."
  exit 1
fi
echo "    Packaged: ${CHART_TGZ}"

# --- Copy air-gapped values file ---
echo "--> Copying values-airgapped.yaml..."
cp "${CHART_DIR}/values-airgapped.yaml" "${BUNDLE_DIR}/values-airgapped.yaml"

# --- Copy docs ---
echo "--> Copying docs..."
if [ -d "docs" ]; then
  cp -r docs "${BUNDLE_DIR}/docs"
  echo "    Copied: docs/ ($(find docs -type f | wc -l | tr -d ' ') files)"
else
  echo "    WARNING: docs/ directory not found, skipping."
fi

# --- Generate checksums ---
echo "--> Generating checksums..."
(
  cd "${BUNDLE_DIR}"
  ${SHA_CMD} \
    ifa-control-plane.tar \
    ifa-node-agent.tar \
    "$(basename "${CHART_TGZ}")" \
    values-airgapped.yaml \
    > checksums.sha256
)
echo "    Written: ${BUNDLE_DIR}/checksums.sha256"
cat "${BUNDLE_DIR}/checksums.sha256"
echo ""

# --- Create final tarball ---
echo "--> Creating ${OUTPUT_TARBALL}..."
tar -czf "${OUTPUT_TARBALL}" "${BUNDLE_DIR}"
BUNDLE_SIZE=$(du -sh "${OUTPUT_TARBALL}" | cut -f1)
echo "    Created: ${OUTPUT_TARBALL} (${BUNDLE_SIZE})"

# --- Summary ---
echo ""
echo "==> Bundle contents:"
find "${BUNDLE_DIR}" -type f | sort | sed 's/^/    /'
echo ""
echo "==> Done. Transfer ${OUTPUT_TARBALL} to your restricted environment."
echo "    Verify checksums after transfer:"
echo "    tar -xzf ${OUTPUT_TARBALL} && cd ${BUNDLE_DIR} && ${SHA_CMD} -c checksums.sha256"

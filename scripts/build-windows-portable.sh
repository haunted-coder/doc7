#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-dev}"
OUTPUT_DIR="${2:-${ROOT_DIR}/dist/portable/${VERSION}}"
TOOLS_DIR="${DOC7_WINDOWS_TOOLS_DIR:-}"

if [[ "${OUTPUT_DIR}" != /* ]]; then
  OUTPUT_DIR="$(pwd)/${OUTPUT_DIR}"
fi

case "${VERSION}" in
  *[!A-Za-z0-9._-]*)
    echo "version may only contain letters, numbers, dots, underscores, and hyphens" >&2
    exit 1
    ;;
esac

if [[ -z "${TOOLS_DIR}" ]]; then
  echo "DOC7_WINDOWS_TOOLS_DIR must point to a Windows tools directory" >&2
  echo "Expected tools/mupdf/mutool.exe and tools/LibreOfficePortable/App/libreoffice/program/soffice.exe" >&2
  exit 1
fi

if [[ ! -d "${TOOLS_DIR}" ]]; then
  echo "Windows tools directory does not exist: ${TOOLS_DIR}" >&2
  exit 1
fi

for required in \
  "mupdf/mutool.exe" \
  "LibreOfficePortable/App/libreoffice/program/soffice.exe"; do
  if [[ ! -f "${TOOLS_DIR}/${required}" ]]; then
    echo "portable bundle is missing tools/${required}" >&2
    exit 1
  fi
done

if [[ -e "${OUTPUT_DIR}" ]]; then
  echo "portable release output already exists: ${OUTPUT_DIR}" >&2
  exit 1
fi

BUILD_COMMIT="${DOC7_BUILD_COMMIT:-unknown}"
BUILD_DATE="${DOC7_BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/doc7-windows-portable.XXXXXXXX")"
cleanup() {
  rm -rf -- "${STAGE_DIR}"
}
trap cleanup EXIT

PACKAGE_DIR="${STAGE_DIR}/doc7"
mkdir -p "${PACKAGE_DIR}/examples"

LDFLAGS="-s -w \
  -X github.com/magicrew/doc7/internal/cli.buildVersion=${VERSION} \
  -X github.com/magicrew/doc7/internal/cli.buildCommit=${BUILD_COMMIT} \
  -X github.com/magicrew/doc7/internal/cli.buildDate=${BUILD_DATE}"

cd "${ROOT_DIR}"
env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags="${LDFLAGS}" \
  -o "${PACKAGE_DIR}/doc7.exe" ./cmd/doc7

cp "${ROOT_DIR}/LICENSE" "${PACKAGE_DIR}/LICENSE"
cp "${ROOT_DIR}/README.md" "${PACKAGE_DIR}/README.md"
cp "${ROOT_DIR}/README.zh-CN.md" "${PACKAGE_DIR}/README.zh-CN.md"
cp "${ROOT_DIR}/packaging/windows/"* "${PACKAGE_DIR}/"
cp -R "${ROOT_DIR}/examples/visual-report" "${PACKAGE_DIR}/examples/"
cp -R "${ROOT_DIR}/examples/format-parity" "${PACKAGE_DIR}/examples/"
cp -R "${TOOLS_DIR}" "${PACKAGE_DIR}/tools"

mkdir -p "${OUTPUT_DIR}"
ARCHIVE="${OUTPUT_DIR}/doc7_${VERSION}_windows_amd64_portable.zip"
(
  cd "${STAGE_DIR}"
  zip -qr -0 "${ARCHIVE}" doc7
)

(
  cd "${OUTPUT_DIR}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$(basename "${ARCHIVE}")" > checksums.txt
  else
    shasum -a 256 "$(basename "${ARCHIVE}")" > checksums.txt
  fi
)

echo "portable Windows artifact: ${ARCHIVE}"

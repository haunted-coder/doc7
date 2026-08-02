#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-dev}"
OUTPUT_DIR="${2:-${ROOT_DIR}/dist/releases/${VERSION}}"
if [[ "${OUTPUT_DIR}" != /* ]]; then
  OUTPUT_DIR="$(pwd)/${OUTPUT_DIR}"
fi

case "${VERSION}" in
  *[!A-Za-z0-9._-]*)
    echo "version may only contain letters, numbers, dots, underscores, and hyphens" >&2
    exit 1
    ;;
esac

if [[ -e "${OUTPUT_DIR}" ]]; then
  echo "release output already exists: ${OUTPUT_DIR}" >&2
  exit 1
fi

BUILD_COMMIT="${DOC7_BUILD_COMMIT:-unknown}"
BUILD_DATE="${DOC7_BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/doc7-release.XXXXXXXX")"
cleanup() {
  rm -rf -- "${STAGE_DIR}"
}
trap cleanup EXIT
mkdir -p "${OUTPUT_DIR}"

LDFLAGS="-s -w \
  -X github.com/magicrew/doc7/internal/cli.buildVersion=${VERSION} \
  -X github.com/magicrew/doc7/internal/cli.buildCommit=${BUILD_COMMIT} \
  -X github.com/magicrew/doc7/internal/cli.buildDate=${BUILD_DATE}"

build_target() {
  local goos="$1"
  local goarch="$2"
  local name="doc7_${VERSION}_${goos}_${goarch}"
  local package_dir="${STAGE_DIR}/${name}"
  local binary="doc7"

  if [[ "${goos}" == "windows" ]]; then
    binary="doc7.exe"
  fi

  mkdir -p "${package_dir}"
  env CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags="${LDFLAGS}" \
    -o "${package_dir}/${binary}" "${ROOT_DIR}/cmd/doc7"
  cp "${ROOT_DIR}/LICENSE" "${package_dir}/LICENSE"
  cp "${ROOT_DIR}/packaging/cli/README.txt" "${package_dir}/README.txt"

  if [[ "${goos}" == "windows" ]]; then
    cp "${ROOT_DIR}/packaging/windows/"* "${package_dir}/"
    (
      cd "${STAGE_DIR}"
      zip -qr "${OUTPUT_DIR}/${name}.zip" "${name}"
    )
  else
    tar -C "${STAGE_DIR}" -czf "${OUTPUT_DIR}/${name}.tar.gz" "${name}"
  fi
}

cd "${ROOT_DIR}"
build_target darwin amd64
build_target darwin arm64
build_target linux amd64
build_target linux arm64
build_target windows amd64
build_target windows arm64

if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "${OUTPUT_DIR}"
    sha256sum ./*.tar.gz ./*.zip > checksums.txt
  )
else
  (
    cd "${OUTPUT_DIR}"
    shasum -a 256 ./*.tar.gz ./*.zip > checksums.txt
  )
fi

echo "release artifacts: ${OUTPUT_DIR}"

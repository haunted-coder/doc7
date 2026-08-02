#!/usr/bin/env bash

set -euo pipefail

readonly MAX_BLOB_BYTES=$((5 * 1024 * 1024))
readonly MAX_TOTAL_BYTES=$((25 * 1024 * 1024))
readonly LFS_HEADER="version https://git-lfs.github.com/spec/v1"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "repository size check must run inside a Git work tree" >&2
  exit 1
fi

is_lfs_pointer() {
  local tracked_path="$1"
  [[ "$(git show ":${tracked_path}" 2>/dev/null | head -n 1)" == "${LFS_HEADER}" ]]
}

requires_lfs() {
  local tracked_path
  tracked_path="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "${tracked_path}" in
    *.pdf|*.docx|*.pptx|*.xlsx|*.tif|*.tiff|*.zip)
      return 0
      ;;
  esac
  return 1
}

is_forbidden() {
  local tracked_path
  tracked_path="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "${tracked_path}" in
    dist/*|output/*|tools/*|doc7-server/*|*/tools/libreofficeportable/*|*/tools/mupdf/*|*.exe|*.dll|*.dylib|*.so|*.tar|*.tar.gz)
      return 0
      ;;
  esac
  return 1
}

total_bytes=0
failed=0

while IFS= read -r -d '' tracked_path; do
  if is_forbidden "${tracked_path}"; then
    echo "forbidden generated or executable file is tracked: ${tracked_path}" >&2
    failed=1
    continue
  fi

  blob_bytes="$(git cat-file -s ":${tracked_path}")"

  if requires_lfs "${tracked_path}"; then
    if ! is_lfs_pointer "${tracked_path}"; then
      echo "binary document must be stored with Git LFS: ${tracked_path}" >&2
      failed=1
    fi
    continue
  fi

  if (( blob_bytes > MAX_BLOB_BYTES )); then
    echo "Git blob exceeds 5 MiB: ${tracked_path} (${blob_bytes} bytes)" >&2
    failed=1
  fi

  total_bytes=$((total_bytes + blob_bytes))
done < <(git ls-files -z)

if (( total_bytes > MAX_TOTAL_BYTES )); then
  echo "ordinary Git blobs exceed 25 MiB (${total_bytes} bytes)" >&2
  failed=1
fi

if (( failed != 0 )); then
  exit 1
fi

printf 'repository size policy: ordinary Git blobs %d bytes\n' "${total_bytes}"

#!/usr/bin/env bash

set -euo pipefail

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "public source check must run inside a Git work tree" >&2
  exit 1
fi

credential_pattern='gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----'
if git grep -I -q -E "${credential_pattern}" -- .; then
  echo "tracked source contains a credential or private-key pattern" >&2
  exit 1
fi

escaped_ipv4_pattern='[0-9]{1,3}\\\.[0-9]{1,3}\\\.[0-9]{1,3}\\\.[0-9]{1,3}'
if git grep -I -q -E "${escaped_ipv4_pattern}" -- .; then
  echo "tracked source contains an escaped IPv4 address" >&2
  exit 1
fi

unexpected_ipv4="$({ git grep -I -h -o -E '([0-9]{1,3}\.){3}[0-9]{1,3}' -- . || true; } \
  | sort -u \
  | grep -E -v '^(0[.]0[.]0[.]0|127[.]0[.]0[.]1)$' || true)"
if [[ -n "${unexpected_ipv4}" ]]; then
  echo "tracked source contains a non-local IPv4 address:" >&2
  printf '%s\n' "${unexpected_ipv4}" >&2
  exit 1
fi

if git grep -I -q -E '/Users/[A-Za-z0-9._-]+/|[A-Za-z]:\\Users\\[^<%\\]+' -- .; then
  echo "tracked source contains a user-specific absolute path" >&2
  exit 1
fi

expected_repository='github.com/magicrew/doc7'
unexpected_repositories="$({ git grep -I -h -o -E 'github\.com/[A-Za-z0-9_.-]+/doc7' -- . || true; } \
  | sort -u \
  | grep -F -x -v "${expected_repository}" || true)"
if [[ -n "${unexpected_repositories}" ]]; then
  echo "tracked source references an unexpected doc7 repository namespace:" >&2
  printf '%s\n' "${unexpected_repositories}" >&2
  exit 1
fi

echo "public source policy: clean"

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(tr -d '[:space:]' < "${ROOT_DIR}/VERSION")"
CHANGELOG_JSON="${ROOT_DIR}/releases/changelog.json"

if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid VERSION: ${VERSION}" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

jq -e '.schemaVersion == "baize.mcp.changelog.v1" and (.entries | type == "array")' "${CHANGELOG_JSON}" >/dev/null

if [[ -f "${ROOT_DIR}/releases/latest.json" ]]; then
  jq -e --arg version "${VERSION}" '
    .schemaVersion == "baize.mcp.release.v1" and
    .version == $version and
    .tag == ("v" + $version) and
    .channel == "stable" and
    .minimumBaizeVersion == "0.2.1" and
    .transport == "stdio" and
    (.assets | length == 6) and
    ([.assets[] | (.os + "/" + .arch)] | unique | length == 6)
  ' "${ROOT_DIR}/releases/latest.json" >/dev/null
  jq -e --arg version "${VERSION}" '
    any(.entries[]; .version == $version and .tag == ("v" + $version) and .channel == "stable")
  ' "${CHANGELOG_JSON}" >/dev/null
  grep -q "^## ${VERSION} - " "${ROOT_DIR}/CHANGELOG.md"
fi

grep -q 'baize_connection_status' "${ROOT_DIR}/README.md"
grep -q 'baize_agents_list' "${ROOT_DIR}/README.en.md"

if [[ -d "${ROOT_DIR}/dist" ]]; then
  jq -e --arg version "${VERSION}" '
    .schemaVersion == "baize.mcp.release-assets.v1" and
    .version == $version and
    .tag == ("v" + $version) and
    (.assets | length == 6)
  ' "${ROOT_DIR}/dist/release-assets.json" >/dev/null
  (
    cd "${ROOT_DIR}/dist"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum -c SHA256SUMS >/dev/null
    else
      shasum -a 256 -c SHA256SUMS >/dev/null
    fi
  )

  for archive in "${ROOT_DIR}"/dist/baize-mcp_*.tar.gz; do
    [[ -e "${archive}" ]] || continue
    archive_entries="$(tar -tzf "${archive}")"
    if ! grep -Eq '(^|/)baize-mcp\.sha256$' <<<"${archive_entries}"; then
      echo "release archive is missing baize-mcp.sha256: ${archive}" >&2
      exit 1
    fi
  done
  for archive in "${ROOT_DIR}"/dist/baize-mcp_*.zip; do
    [[ -e "${archive}" ]] || continue
    if ! command -v unzip >/dev/null 2>&1; then
      echo "unzip is required to verify Windows release archives" >&2
      exit 1
    fi
    archive_entries="$(unzip -Z1 "${archive}")"
    if ! grep -Eq '(^|/)baize-mcp\.sha256$' <<<"${archive_entries}"; then
      echo "release archive is missing baize-mcp.sha256: ${archive}" >&2
      exit 1
    fi
  done
fi

echo "release metadata verified for v${VERSION}"

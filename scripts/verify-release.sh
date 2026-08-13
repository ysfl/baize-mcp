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

sha256_stream() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    shasum -a 256 | awk '{print $1}'
  fi
}

verify_archive_checksum() {
  local archive="$1"
  local binary_name="$2"
  local metadata expected expected_name actual
  metadata="$("$3" "$archive" baize-mcp.sha256)"
  expected="$(awk 'NR == 1 {print $1}' <<<"${metadata}")"
  expected_name="$(awk 'NR == 1 {print $2}' <<<"${metadata}")"
  actual="$("$4" "$archive" "${binary_name}" | sha256_stream)"
  if [[ ! "${expected}" =~ ^[0-9a-fA-F]{64}$ || "${expected_name}" != "${binary_name}" || "${expected}" != "${actual}" ]]; then
    echo "release archive executable checksum is invalid: ${archive}" >&2
    exit 1
  fi
}

extract_tar_file() {
  tar -xOzf "$1" "$2"
}

extract_zip_file() {
  unzip -p "$1" "$2"
}

compare_versions() {
  awk -v left="$1" -v right="$2" 'BEGIN {
    split(left, l, "."); split(right, r, ".");
    for (i = 1; i <= 3; i++) {
      if ((l[i] + 0) < (r[i] + 0)) { print -1; exit }
      if ((l[i] + 0) > (r[i] + 0)) { print 1; exit }
    }
    print 0
  }'
}

verify_latest_metadata() {
  local latest_path="$1"
  jq -e '
    .schemaVersion == "baize.mcp.release.v1" and
    (.version | type == "string") and
    (.tag | type == "string") and
    .channel == "stable" and
    .minimumBaizeVersion == "0.2.1" and
    .transport == "stdio" and
    (.assets | length == 6) and
    ([.assets[] | (.os + "/" + .arch)] | unique | length == 6)
  ' "${latest_path}" >/dev/null
}

jq -e '.schemaVersion == "baize.mcp.changelog.v1" and (.entries | type == "array")' "${CHANGELOG_JSON}" >/dev/null

if [[ -f "${ROOT_DIR}/releases/latest.json" ]]; then
  verify_latest_metadata "${ROOT_DIR}/releases/latest.json"
  latest_version="$(jq -r '.version' "${ROOT_DIR}/releases/latest.json")"
  latest_tag="$(jq -r '.tag' "${ROOT_DIR}/releases/latest.json")"
  [[ "${latest_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "latest.json has an invalid version" >&2; exit 1; }
  [[ "${latest_tag}" == "v${latest_version}" ]] || { echo "latest.json tag does not match its version" >&2; exit 1; }
  relation="$(compare_versions "${latest_version}" "${VERSION}")"
  if [[ "${relation}" == "1" ]]; then
    echo "latest.json is newer than VERSION" >&2
    exit 1
  elif [[ "${relation}" == "0" ]]; then
    jq -e --arg version "${VERSION}" '
      any(.entries[]; .version == $version and .tag == ("v" + $version) and .channel == "stable")
    ' "${CHANGELOG_JSON}" >/dev/null
    grep -q "^## ${VERSION} - " "${ROOT_DIR}/CHANGELOG.md"
  else
    grep -q '^## Unreleased$' "${ROOT_DIR}/CHANGELOG.md"
    if jq -e --arg version "${VERSION}" '
      any(.entries[]; .version == $version and .tag == ("v" + $version) and .channel == "stable")
    ' "${CHANGELOG_JSON}" >/dev/null; then
      echo "candidate VERSION is already recorded as stable" >&2
      exit 1
    fi
    [[ ! -f "${ROOT_DIR}/releases/v${VERSION}.md" ]] || { echo "candidate release notes already exist" >&2; exit 1; }
  fi
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
  if [[ -f "${ROOT_DIR}/dist/latest.json" ]]; then
    verify_latest_metadata "${ROOT_DIR}/dist/latest.json"
    jq -e --arg version "${VERSION}" '
      .version == $version and .tag == ("v" + $version)
    ' "${ROOT_DIR}/dist/latest.json" >/dev/null
  fi
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
    verify_archive_checksum "${archive}" baize-mcp extract_tar_file extract_tar_file
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
    verify_archive_checksum "${archive}" baize-mcp.exe extract_zip_file extract_zip_file
  done
fi

echo "release metadata verified for v${VERSION}"

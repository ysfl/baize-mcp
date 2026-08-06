#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
VERSION="$(tr -d '[:space:]' < "${ROOT_DIR}/VERSION")"

if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid VERSION: ${VERSION}" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

cd "${ROOT_DIR}"

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TEMP_DIR}"' EXIT
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

asset_objects=()
targets=(
  "linux amd64 tar.gz"
  "linux arm64 tar.gz"
  "darwin amd64 tar.gz"
  "darwin arm64 tar.gz"
  "windows amd64 zip"
  "windows arm64 zip"
)

for target in "${targets[@]}"; do
  read -r target_os target_arch archive_format <<<"${target}"
  package_name="baize-mcp_${VERSION}_${target_os}_${target_arch}"
  stage_dir="${TEMP_DIR}/${package_name}"
  mkdir -p "${stage_dir}"

  binary_name="baize-mcp"
  if [[ "${target_os}" == "windows" ]]; then
    binary_name="baize-mcp.exe"
  fi
  CGO_ENABLED=0 GOOS="${target_os}" GOARCH="${target_arch}" \
    go build -trimpath \
    -ldflags "-s -w -X github.com/ysfl/baize-mcp/internal/buildinfo.Version=${VERSION} -X github.com/ysfl/baize-mcp/internal/buildinfo.ReleaseSelfCheck=required" \
    -o "${stage_dir}/${binary_name}" ./cmd/baize-mcp
  cp "${ROOT_DIR}/LICENSE" "${ROOT_DIR}/README.md" "${ROOT_DIR}/README.en.md" "${ROOT_DIR}/VERSION" "${stage_dir}/"
  binary_checksum="$(sha256_file "${stage_dir}/${binary_name}")"
  printf '%s  %s\n' "${binary_checksum}" "${binary_name}" > "${stage_dir}/baize-mcp.sha256"

  notices_file="${stage_dir}/THIRD_PARTY_NOTICES.md"
  licenses_dir="${stage_dir}/third-party-licenses"
  mkdir -p "${licenses_dir}"
  printf '%s\n\n' '# Third-Party Notices' 'This archive includes the following Go modules. Their license texts are provided in `third-party-licenses/`.' > "${notices_file}"
  while IFS=$'\t' read -r module_path module_version module_dir; do
    license_file="$(find "${module_dir}" -maxdepth 1 -type f \( -iname 'LICENSE' -o -iname 'LICENSE.txt' -o -iname 'COPYING' \) | LC_ALL=C sort | head -n 1)"
    if [[ -z "${license_file}" ]]; then
      echo "license file not found for ${module_path}@${module_version}" >&2
      exit 1
    fi
    license_name="${module_path//\//_}@${module_version}.txt"
    cp "${license_file}" "${licenses_dir}/${license_name}"
    printf -- '- `%s@%s`\n' "${module_path}" "${module_version}" >> "${notices_file}"
  done < <(
    CGO_ENABLED=0 GOOS="${target_os}" GOARCH="${target_arch}" go list -deps -json ./cmd/baize-mcp |
      jq -r 'select(.Module != null and .Module.Main != true) | [.Module.Path, .Module.Version, .Module.Dir] | @tsv' |
      LC_ALL=C sort -u
  )

  if [[ "${archive_format}" == "zip" ]]; then
    archive_name="${package_name}.zip"
    (
      cd "${stage_dir}"
      zip -q -X -r "${DIST_DIR}/${archive_name}" "${binary_name}" baize-mcp.sha256 LICENSE README.md README.en.md VERSION THIRD_PARTY_NOTICES.md third-party-licenses
    )
  else
    archive_name="${package_name}.tar.gz"
    tar -C "${stage_dir}" -czf "${DIST_DIR}/${archive_name}" "${binary_name}" baize-mcp.sha256 LICENSE README.md README.en.md VERSION THIRD_PARTY_NOTICES.md third-party-licenses
  fi

  checksum="$(sha256_file "${DIST_DIR}/${archive_name}")"
  asset_objects+=("$(jq -n \
    --arg name "${archive_name}" \
    --arg os "${target_os}" \
    --arg arch "${target_arch}" \
    --arg format "${archive_format}" \
    --arg sha256 "${checksum}" \
    '{name: $name, os: $os, arch: $arch, format: $format, sha256: $sha256}')")
done

assets_json="$(printf '%s\n' "${asset_objects[@]}" | jq -s '.')"
jq -n \
  --arg version "${VERSION}" \
  --arg tag "v${VERSION}" \
  --arg generated_at "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
  --argjson assets "${assets_json}" \
  '{schemaVersion: "baize.mcp.release-assets.v1", version: $version, tag: $tag, generatedAt: $generated_at, assets: $assets}' \
  > "${DIST_DIR}/release-assets.json"

if [[ -f "${ROOT_DIR}/releases/latest.json" ]]; then
  cp "${ROOT_DIR}/releases/latest.json" "${DIST_DIR}/latest.json"
fi

(
  cd "${DIST_DIR}"
  : > SHA256SUMS
  while IFS= read -r file; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "${file}" >> SHA256SUMS
    else
      shasum -a 256 "${file}" >> SHA256SUMS
    fi
  done < <(find . -maxdepth 1 -type f ! -name SHA256SUMS -print | sed 's#^./##' | LC_ALL=C sort)
)

echo "release assets written to ${DIST_DIR}"

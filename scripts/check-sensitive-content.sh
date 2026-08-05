#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELF_PATH="scripts/check-sensitive-content.sh"
patterns=(
  'BEGIN [A-Z ]*PRIVATE KEY'
  'gh[pousr]_[A-Za-z0-9_]{20,}'
  'github_pat_[A-Za-z0-9_]{20,}'
  '(AKIA|ASIA)[0-9A-Z]{16}'
  'https?://[^/@:[:space:]]+:[^/@[:space:]]+@'
  'Authorization:[[:space:]]*Bearer[[:space:]]+[A-Za-z0-9._-]{20,}'
)

failed=0
while IFS= read -r file; do
  [[ "${file}" == "${SELF_PATH}" ]] && continue
  [[ -f "${ROOT_DIR}/${file}" ]] || continue
  grep -Iq . "${ROOT_DIR}/${file}" || continue
  for pattern in "${patterns[@]}"; do
    if grep -qE "${pattern}" "${ROOT_DIR}/${file}"; then
      echo "potential sensitive value found in ${file}" >&2
      failed=1
      break
    fi
  done
done < <(git -C "${ROOT_DIR}" ls-files --cached --others --exclude-standard)

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi
echo "sensitive content check passed"

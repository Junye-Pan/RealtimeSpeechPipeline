#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

ALLOWLIST_REGEX='^\.codex/(skills|rules)/'

tracked_codex_paths=()
while IFS= read -r path; do
  tracked_codex_paths+=("${path}")
done < <(git ls-files .codex)

violations=()

for path in "${tracked_codex_paths[@]}"; do
  if [[ ! "${path}" =~ ${ALLOWLIST_REGEX} ]]; then
    violations+=("${path}")
  fi
done

if (( ${#violations[@]} > 0 )); then
  echo "codex-artifact-policy-check: FAIL"
  echo "Tracked .codex paths outside allowlist (.codex/skills/**, .codex/rules/**):"
  printf '  - %s\n' "${violations[@]}"
  echo "Remediation: untrack generated artifacts, e.g. \`git rm --cached <path>\`."
  exit 1
fi

echo "codex-artifact-policy-check: PASS"

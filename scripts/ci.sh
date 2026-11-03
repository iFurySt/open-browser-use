#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
python_bin="${PYTHON:-}"
if [[ -z "${python_bin}" ]]; then
  if command -v python >/dev/null 2>&1; then
    python_bin="python"
  elif command -v python3 >/dev/null 2>&1; then
    python_bin="python3"
  else
    echo "python or python3 is required" >&2
    exit 127
  fi
fi

"${repo_root}/scripts/check-docs.sh"
"${repo_root}/scripts/check-repo-hygiene.sh"
"${repo_root}/scripts/check-action-pinning.sh"
node "${repo_root}/scripts/generate-chrome-extension-icons.mjs"
"${repo_root}/scripts/package-chrome-extension.sh" >/dev/null
"${repo_root}/scripts/package-zen-extension.sh" >/dev/null
"${repo_root}/scripts/package-skill.sh" >/dev/null
(
  cd "${repo_root}"
  go test ./...
  node --test apps/chrome-extension/*.test.mjs
  node --test apps/zen-extension/*.test.mjs
  node --test scripts/*.test.mjs
  pnpm -r --if-present test
)
(
  cd "${repo_root}/packages/open-browser-use-python"
  "${python_bin}" -m unittest
)
node --check "${repo_root}/scripts/chrome-web-store-oauth.mjs"
node --check "${repo_root}/scripts/generate-chrome-extension-icons.mjs"
node --check "${repo_root}/scripts/package-chrome-extension-crx.mjs"
node --check "${repo_root}/scripts/publish-chrome-web-store.mjs"

while IFS= read -r file; do
  bash -n "$file"
done < <(find "${repo_root}/scripts" -type f -name '*.sh' | sort)

echo "基础 CI 检查通过"

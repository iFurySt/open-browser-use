#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${repo_root}/dist"
version="${OPEN_BROWSER_USE_VERSION:-$(node -p "require('${repo_root}/packages/open-browser-use-cli/package.json').version")}"

rm -rf "${dist_dir}"
mkdir -p "${dist_dir}"

cli_dir="${dist_dir}/cli"
"${repo_root}/scripts/build-cli-release-archives.sh" "${version}" "${cli_dir}"
chrome_extension_zip="$("${repo_root}/scripts/package-chrome-extension.sh")"
chrome_extension_artifact="$(basename "${chrome_extension_zip}")"
if [[ "${chrome_extension_artifact}" != "open-browser-use-chrome-extension-${version}.zip" ]]; then
  echo "Unexpected Chrome Web Store zip artifact: ${chrome_extension_artifact}" >&2
  exit 1
fi
chrome_extension_crx="$(find "${repo_root}/dist/chrome-extension" -maxdepth 1 -type f -name '*.crx' -print -quit)"
chrome_extension_crx_artifact="$(basename "${chrome_extension_crx}")"
zen_extension_xpi="$("${repo_root}/scripts/package-zen-extension.sh")"
zen_extension_artifact="$(basename "${zen_extension_xpi}")"
if [[ "${zen_extension_artifact}" != "open-browser-use-zen-extension-${version}.xpi" ]]; then
  echo "Unexpected Zen extension artifact: ${zen_extension_artifact}" >&2
  exit 1
fi
skill_zip="$("${repo_root}/scripts/package-skill.sh" | sed -n '1p')"
skill_artifact="$(basename "${skill_zip}")"
skill_bundle="$(find "${repo_root}/dist/skills" -maxdepth 1 -type f -name '*.skill' -print -quit)"
if [[ -z "${skill_bundle}" ]]; then
  echo "Open Browser Use skill bundle not found" >&2
  exit 1
fi
skill_bundle_artifact="$(basename "${skill_bundle}")"

cat > "${dist_dir}/release-manifest.json" <<EOF
{
  "repository": "${GITHUB_REPOSITORY:-local}",
  "git_sha": "${GITHUB_SHA:-$(git -C "${repo_root}" rev-parse HEAD 2>/dev/null || echo unknown)}",
  "generated_at_utc": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "artifacts": {
    "repo_metadata": "repo-metadata.tgz",
    "cli_darwin_amd64": "cli/open-browser-use-cli-${version}-darwin-amd64.tar.gz",
    "cli_darwin_arm64": "cli/open-browser-use-cli-${version}-darwin-arm64.tar.gz",
    "cli_linux_amd64": "cli/open-browser-use-cli-${version}-linux-amd64.tar.gz",
    "cli_linux_arm64": "cli/open-browser-use-cli-${version}-linux-arm64.tar.gz",
    "cli_windows_amd64": "cli/open-browser-use-cli-${version}-windows-amd64.zip",
    "cli_windows_arm64": "cli/open-browser-use-cli-${version}-windows-arm64.zip",
    "chrome_extension": "chrome-extension/${chrome_extension_artifact}",
    "chrome_extension_internal_crx": "chrome-extension/${chrome_extension_crx_artifact}",
    "zen_extension": "zen-extension/${zen_extension_artifact}",
    "open_browser_use_skill_zip": "skills/${skill_artifact}",
    "open_browser_use_skill": "skills/${skill_bundle_artifact}"
  }
}
EOF

tar -czf "${dist_dir}/repo-metadata.tgz" \
  -C "${repo_root}" \
  AGENTS.md README.md CONTRIBUTING.md LICENSE docs scripts .github Makefile

echo "${dist_dir}/repo-metadata.tgz"
find "${cli_dir}" -maxdepth 1 -type f -name '*.tar.gz' | sort
find "${cli_dir}" -maxdepth 1 -type f -name '*.zip' | sort
echo "${chrome_extension_zip}"
echo "${zen_extension_xpi}"
echo "${skill_zip}"
echo "${skill_bundle}"

#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
extension_dir="${repo_root}/apps/zen-extension"
shared_dir="${repo_root}/apps/chrome-extension"
dist_dir="${repo_root}/dist/zen-extension"
manifest_path="${extension_dir}/manifest.json"

if ! command -v node >/dev/null 2>&1; then
  echo "node is required to validate the Zen extension package" >&2
  exit 1
fi

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to package the Zen extension" >&2
  exit 1
fi

version="$(node -e 'const fs=require("fs"); const manifest=JSON.parse(fs.readFileSync(process.argv[1], "utf8")); process.stdout.write(manifest.version);' "${manifest_path}")"
xpi_path="${dist_dir}/open-browser-use-zen-extension-${version}.xpi"

node - "${manifest_path}" "${extension_dir}" "${shared_dir}" <<'NODE'
const fs = require("fs");
const path = require("path");

const manifestPath = process.argv[2];
const extensionDir = process.argv[3];
const sharedDir = process.argv[4];
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const sharedManifest = JSON.parse(fs.readFileSync(path.join(sharedDir, "manifest.json"), "utf8"));
const errors = [];

if (manifest.manifest_version !== 3) errors.push("manifest_version must be 3");
if (manifest.version !== sharedManifest.version) errors.push("Zen and Chrome extension versions must match");
if (manifest.browser_specific_settings?.gecko?.id !== "open-browser-use@ifuryst.com") {
  errors.push("browser_specific_settings.gecko.id must be stable");
}
if (!manifest.background?.scripts?.includes("firefox-compat.js")) {
  errors.push("background.scripts must load firefox-compat.js");
}
if (manifest.permissions?.includes("debugger") || manifest.permissions?.includes("tabGroups")) {
  errors.push("Zen manifest must not request unsupported Chrome permissions");
}
for (const file of ["manifest.json", "firefox-compat.js"]) {
  if (!fs.existsSync(path.join(extensionDir, file))) errors.push(`missing Zen extension file: ${file}`);
}
for (const file of ["background.js", "content-cursor.js", "popup.html", "popup.css", "popup.js"]) {
  if (!fs.existsSync(path.join(sharedDir, file))) errors.push(`missing shared extension file: ${file}`);
}
if (errors.length > 0) {
  for (const error of errors) console.error(error);
  process.exit(1);
}
NODE

node --check "${extension_dir}/firefox-compat.js" >&2
node --check "${shared_dir}/background.js" >&2

rm -rf "${dist_dir}"
staging_dir="${dist_dir}/extension-staging"
mkdir -p "${staging_dir}/icons" "${staging_dir}/images"

cp "${manifest_path}" "${staging_dir}/manifest.json"
cp "${extension_dir}/firefox-compat.js" "${staging_dir}/firefox-compat.js"
for file in background.js content-cursor.js popup.html popup.css popup.js; do
  cp "${shared_dir}/${file}" "${staging_dir}/${file}"
done
for size in 16 32 48 128; do
  cp "${shared_dir}/icons/icon-${size}.png" "${staging_dir}/icons/icon-${size}.png"
done
cp "${shared_dir}/images/cursor-chat.png" "${staging_dir}/images/cursor-chat.png"

(
  cd "${staging_dir}"
  zip -q -r "${xpi_path}" .
)
rm -rf "${staging_dir}"

node - "${manifest_path}" "${xpi_path}" "${dist_dir}/package-manifest.json" <<'NODE'
const crypto = require("crypto");
const fs = require("fs");
const manifest = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const artifact = fs.readFileSync(process.argv[3]);
fs.writeFileSync(process.argv[4], `${JSON.stringify({
  name: manifest.name,
  version: manifest.version,
  extensionId: manifest.browser_specific_settings.gecko.id,
  artifact: process.argv[3],
  sha256: crypto.createHash("sha256").update(artifact).digest("hex"),
  generatedAtUtc: new Date().toISOString()
}, null, 2)}\n`);
NODE

echo "${xpi_path}"

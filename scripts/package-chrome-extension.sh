#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
extension_dir="${repo_root}/apps/chrome-extension"
dist_dir="${repo_root}/dist/chrome-extension"

if ! command -v node >/dev/null 2>&1; then
  echo "node is required to validate the Chrome extension package" >&2
  exit 1
fi

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to package the Chrome extension" >&2
  exit 1
fi

manifest_path="${extension_dir}/manifest.json"
version="$(node -e 'const fs=require("fs"); const manifest=JSON.parse(fs.readFileSync(process.argv[1], "utf8")); process.stdout.write(manifest.version);' "${manifest_path}")"
package_name="open-browser-use-chrome-extension-${version}.zip"
zip_path="${dist_dir}/${package_name}"
crx_name="open-browser-use-chrome-extension-${version}.crx"
crx_path="${dist_dir}/${crx_name}"

node - "${manifest_path}" "${extension_dir}" <<'NODE'
const fs = require("fs");
const path = require("path");

const manifestPath = process.argv[2];
const extensionDir = process.argv[3];
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));

const requiredFiles = [
  "manifest.json",
  "background.js",
  "content-cursor.js",
  "content-webmcp-main.js",
  "content-webmcp-bridge.js",
  "icons/icon-16.png",
  "icons/icon-32.png",
  "icons/icon-48.png",
  "icons/icon-128.png",
  "images/cursor-chat.png",
  "popup.css",
  "popup.html",
  "popup.js"
];

const errors = [];
if (manifest.manifest_version !== 3) {
  errors.push("manifest_version must be 3");
}
if (manifest.background?.service_worker !== "background.js") {
  errors.push("background.service_worker must be background.js");
}
if (!manifest.permissions?.includes("nativeMessaging")) {
  errors.push("permissions must include nativeMessaging");
}
const webResources = manifest.web_accessible_resources ?? [];
if (
  !webResources.some(
    (entry) =>
      Array.isArray(entry.resources) &&
      entry.resources.includes("images/cursor-chat.png") &&
      Array.isArray(entry.matches) &&
      entry.matches.includes("<all_urls>")
  )
) {
  errors.push("web_accessible_resources must expose images/cursor-chat.png to <all_urls>");
}
for (const size of ["16", "32", "48", "128"]) {
  if (manifest.icons?.[size] !== `icons/icon-${size}.png`) {
    errors.push(`icons.${size} must be icons/icon-${size}.png`);
  }
}
if (manifest.action?.default_icon?.["16"] !== "icons/icon-16.png") {
  errors.push("action.default_icon.16 must be icons/icon-16.png");
}
if (manifest.action?.default_icon?.["32"] !== "icons/icon-32.png") {
  errors.push("action.default_icon.32 must be icons/icon-32.png");
}
if (!/^\d+\.\d+\.\d+(?:\.\d+)?$/.test(manifest.version ?? "")) {
  errors.push("version must use Chrome extension numeric version format");
}
for (const file of requiredFiles) {
  if (!fs.existsSync(path.join(extensionDir, file))) {
    errors.push(`missing extension file: ${file}`);
  }
}
if (errors.length > 0) {
  for (const error of errors) {
    console.error(error);
  }
  process.exit(1);
}
NODE

node --check "${extension_dir}/background.js" >&2
node --check "${extension_dir}/content-cursor.js" >&2
node --check "${extension_dir}/content-webmcp-main.js" >&2
node --check "${extension_dir}/content-webmcp-bridge.js" >&2
node --check "${extension_dir}/popup.js" >&2

rm -rf "${dist_dir}"
mkdir -p "${dist_dir}"
staging_dir="${dist_dir}/extension-staging"
mkdir -p "${staging_dir}"

while IFS= read -r file; do
  mkdir -p "${staging_dir}/$(dirname "${file}")"
  cp "${extension_dir}/${file}" "${staging_dir}/${file}"
done <<'EOF'
manifest.json
background.js
content-cursor.js
content-webmcp-main.js
content-webmcp-bridge.js
icons/icon-16.png
icons/icon-32.png
icons/icon-48.png
icons/icon-128.png
images/cursor-chat.png
popup.css
popup.html
popup.js
EOF

(
  cd "${staging_dir}"
  zip -q -r "${zip_path}" \
    manifest.json \
    background.js \
    content-cursor.js \
    content-webmcp-main.js \
    content-webmcp-bridge.js \
    icons/icon-16.png \
    icons/icon-32.png \
    icons/icon-48.png \
    icons/icon-128.png \
    images/cursor-chat.png \
    popup.css \
    popup.html \
    popup.js
)

rm -rf "${staging_dir}"

node "${repo_root}/scripts/package-chrome-extension-crx.mjs" \
  --zip "${zip_path}" \
  --output "${crx_path}" \
  --metadata "${dist_dir}/crx-manifest.json" \
  >&2

node - "${manifest_path}" "${zip_path}" "${crx_path}" "${dist_dir}/package-manifest.json" <<'NODE'
const crypto = require("crypto");
const fs = require("fs");

const manifestPath = process.argv[2];
const zipPath = process.argv[3];
const crxPath = process.argv[4];
const outputPath = process.argv[5];
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const zip = fs.readFileSync(zipPath);
const crx = fs.readFileSync(crxPath);
const crxManifest = JSON.parse(
  fs.readFileSync(`${require("path").dirname(outputPath)}/crx-manifest.json`, "utf8")
);

const payload = {
  name: manifest.name,
  version: manifest.version,
  artifact: zipPath,
  sha256: crypto.createHash("sha256").update(zip).digest("hex"),
  internalCrxArtifact: crxPath,
  internalCrxSha256: crypto.createHash("sha256").update(crx).digest("hex"),
  crxExtensionId: crxManifest.extensionId,
  crxKeySource: crxManifest.keySource,
  generatedAtUtc: new Date().toISOString()
};

fs.writeFileSync(outputPath, `${JSON.stringify(payload, null, 2)}\n`);
NODE

echo "${zip_path}"

#!/bin/sh
# Builds the release binaries into dist/ and records their checksums for the npm package.
set -eu
cd "$(dirname "$0")/.."

version=$(node -p "require('./npm/package.json').version")
rm -rf dist
mkdir -p dist

for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  os=${target%/*}
  arch=${target#*/}
  name="accred-$os-$arch"
  [ "$os" = windows ] && name="$name.exe"
  echo "building $name"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$version" -o "dist/$name" .
done

node -e '
const fs = require("fs"), crypto = require("crypto");
const sums = {};
for (const file of fs.readdirSync("dist").sort()) {
  sums[file] = crypto.createHash("sha256").update(fs.readFileSync("dist/" + file)).digest("hex");
}
fs.writeFileSync("npm/checksums.json", JSON.stringify(sums, null, 2) + "\n");
'
cp README.md LICENSE npm/
echo "built v$version; checksums written to npm/checksums.json"

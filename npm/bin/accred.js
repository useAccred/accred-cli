#!/usr/bin/env node
// Runs the Accred CLI. The program itself is a native binary published with each
// GitHub release; this script fetches the one for this machine on first use,
// checks it against the checksum shipped in this package, and keeps it in ~/.accred.
"use strict";

const { spawnSync } = require("node:child_process");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const { version } = require("../package.json");
const checksums = require("../checksums.json");

const ASSETS = {
  "darwin-arm64": "accred-darwin-arm64",
  "darwin-x64": "accred-darwin-amd64",
  "linux-x64": "accred-linux-amd64",
  "linux-arm64": "accred-linux-arm64",
  "win32-x64": "accred-windows-amd64.exe",
};

function fail(message) {
  console.error(`accred: ${message}`);
  process.exit(1);
}

async function download(asset, destination) {
  const url = `https://github.com/useAccred/accred-cli/releases/download/v${version}/${asset}`;
  console.error(`Downloading Accred CLI v${version}…`);
  const response = await fetch(url);
  if (!response.ok) fail(`could not download ${url} (HTTP ${response.status})`);
  const data = Buffer.from(await response.arrayBuffer());

  const actual = crypto.createHash("sha256").update(data).digest("hex");
  if (actual !== checksums[asset]) {
    fail(`the download did not match its checksum and was discarded.\n  expected ${checksums[asset]}\n  received ${actual}`);
  }

  fs.mkdirSync(path.dirname(destination), { recursive: true });
  // Write beside the target and rename, so an interrupted download never leaves a broken binary.
  const partial = `${destination}.${process.pid}.partial`;
  fs.writeFileSync(partial, data, { mode: 0o755 });
  fs.renameSync(partial, destination);
}

async function main() {
  const asset = ASSETS[`${process.platform}-${process.arch}`];
  if (!asset || !checksums[asset]) {
    fail(`no build for ${process.platform} ${process.arch} yet. Build from source: https://github.com/useAccred/accred-cli`);
  }
  const binary = path.join(os.homedir(), ".accred", "bin", version, asset);
  if (!fs.existsSync(binary)) await download(asset, binary);

  const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) fail(result.error.message);
  process.exit(result.status ?? 1);
}

main().catch((error) => fail(error.message));

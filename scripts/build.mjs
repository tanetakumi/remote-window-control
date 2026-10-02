import { spawnSync } from "node:child_process";
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const isWindows = process.platform === "win32";
const web = join(root, "web-ui");
const server = join(root, "control-server");
const project = join(root, "window-capture/apps/CaptureProbe/CaptureProbe.csproj");
const dist = join(root, "dist");
const output = join(dist, "share-app");
const env = { ...process.env, GOTOOLCHAIN: "local" };
// Run tests on this machine even if the caller has cross-compilation variables set.
delete env.GOOS;
delete env.GOARCH;

function run(command, args, options = {}) {
  console.log(`\n> ${command} ${args.join(" ")}`);
  const result = spawnSync(command, args, {
    cwd: root,
    env,
    stdio: "inherit",
    // npm is a .cmd shim on Windows; all npm arguments below are fixed literals.
    shell: isWindows && command === "npm",
    ...options,
  });
  if (result.error) throw new Error(`${command}: ${result.error.message}`);
  if (result.status !== 0) {
    throw new Error(`${command} failed (${result.signal ?? `exit ${result.status}`}).`);
  }
  return result;
}

function inspect(command, args, options = {}) {
  return run(command, args, { stdio: ["ignore", "pipe", "inherit"], encoding: "utf8", ...options }).stdout.trim();
}

function requireVersion(name, actual, expected) {
  console.log(`${name}: ${actual}`);
  if (actual !== expected) throw new Error(`${name} ${expected} is required; found ${actual}. Put the pinned SDK on PATH.`);
}

function compareVersions(left, right) {
  for (let index = 0; index < 3; index += 1) {
    if (left[index] !== right[index]) return left[index] - right[index];
  }
  return 0;
}

function requireNodeVersion(actual, range) {
  const match = range.match(/^>=(\d+(?:\.\d+){0,2})\s+<(?<upper>\d+(?:\.\d+){0,2})$/);
  if (!match) throw new Error(`Unsupported Node.js version range in web-ui/package.json: ${range}`);

  const parseBound = (version) => {
    const parts = version.split(".").map(Number);
    while (parts.length < 3) parts.push(0);
    return parts;
  };
  const installed = parseBound(actual);
  const minimum = parseBound(match[1]);
  const maximum = parseBound(match.groups.upper);
  console.log(`Node.js: ${actual} (requires ${range})`);
  if (compareVersions(installed, minimum) < 0 || compareVersions(installed, maximum) >= 0) {
    throw new Error(`Node.js ${actual} does not satisfy web-ui/package.json engines.node (${range}).`);
  }
}

let staging;
try {
  if (process.argv.length > 2) throw new Error("Usage: node scripts/build.mjs");
  const nodeRange = JSON.parse(readFileSync(join(web, "package.json"), "utf8")).engines.node;
  const dotnetVersion = JSON.parse(readFileSync(join(root, "global.json"), "utf8")).sdk.version;
  const goVersion = readFileSync(join(server, "go.mod"), "utf8").match(/^go (\S+)$/m)?.[1];
  requireNodeVersion(process.versions.node, nodeRange);
  inspect("npm", ["--version"]);
  requireVersion(".NET SDK", inspect("dotnet", ["--version"]), dotnetVersion);
  const go = inspect("go", ["version"], { cwd: server });
  requireVersion("Go", go.match(/^go version go(\S+) /)?.[1], goVersion);
  inspect("ffmpeg", ["-version"], { stdio: ["ignore", "pipe", "pipe"] });
  const encoders = inspect("ffmpeg", ["-hide_banner", "-encoders"]);
  if (!/^\s*V\S*\s+libvpx-vp9\s/m.test(encoders)) throw new Error("ffmpeg must support the libvpx-vp9 encoder, as required by CI.");

  mkdirSync(dist, { recursive: true });
  staging = mkdtempSync(join(dist, ".build-"));
  const native = join(staging, "CaptureProbe");
  const host = join(staging, "share-host.exe");

  run("npm", ["ci"], { cwd: web });
  run("npm", ["run", "build"], { cwd: web });
  run("npm", ["test"], { cwd: web });

  const targeting = isWindows ? [] : ["-p:EnableWindowsTargeting=true"];
  run("dotnet", ["restore", project, "--locked-mode", ...targeting]);
  run("dotnet", ["build", project, "-c", "Release", "--no-restore", ...targeting]);
  run("dotnet", ["publish", project, "-c", "Release", "--no-build", "-o", native, ...targeting]);
  if (isWindows) {
    const listed = inspect(join(native, "CaptureProbe.exe"), ["--list"]);
    const windows = JSON.parse(listed);
    if (!Array.isArray(windows)) throw new Error("CaptureProbe --list did not return a JSON array.");
    console.log(`Window enumeration succeeded: ${windows.length} windows`);
  } else {
    console.log("Skipping CaptureProbe --list: this Windows-only smoke test needs Windows.");
  }

  run("go", ["build", "-o", host, "./cmd/share-host"], {
    cwd: server,
    env: { ...env, GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" },
  });
  run("go", ["vet", "./..."], { cwd: server });
  run("go", ["test", "-vet=off", "-count=1", "./..."], { cwd: server });

  // Replace generated assets only; retain any local .env, FFmpeg, and runtime logs.
  mkdirSync(output, { recursive: true });
  for (const directory of ["web", "CaptureProbe"]) rmSync(join(output, directory), { recursive: true, force: true });
  cpSync(join(web, "dist"), join(output, "web"), { recursive: true });
  cpSync(native, join(output, "CaptureProbe"), { recursive: true });
  cpSync(host, join(output, "share-host.exe"));
  cpSync(join(root, "LICENSE"), join(output, "LICENSE"));
  cpSync(join(root, ".github/distribution/はじめにお読みください.txt"), join(output, "はじめにお読みください.txt"));
  try {
    writeFileSync(join(output, ".env"), "SHARE_APP_ADDR=127.0.0.1:8443\n", { flag: "wx" });
  } catch (error) {
    if (error.code !== "EEXIST") throw error;
    console.log("Keeping the existing dist/share-app/.env.");
  }
  console.log(`\nWindows x64 distribution ready: ${output}`);
  console.log("Run share-host.exe on Windows with ffmpeg (libvpx-vp9) available.");
} catch (error) {
  console.error(`\nBuild failed: ${error.message}`);
  process.exitCode = 1;
} finally {
  if (staging) rmSync(staging, { recursive: true, force: true });
}

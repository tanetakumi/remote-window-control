import { spawnSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";

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

export function stageDistribution({ host, native, web, ffmpeg = join(root, "dist/ffmpeg"), output = join(root, "dist/share-app") }) {
  const packageDir = join(ffmpeg, "package");
  const version = readFileSync(join(root, "scripts/ffmpeg/versions.env"), "utf8").match(/^FFMPEG_VERSION=(\S+)$/m)[1];
  const source = join(ffmpeg, `ffmpeg-${version}-share-app-sources.tar.gz`);
  const sourceHash = readFileSync(join(ffmpeg, "sources.sha256"), "utf8").trim().split(/\s+/);
  if (sourceHash[1] !== `ffmpeg-${version}-share-app-sources.tar.gz` ||
    sourceHash[0] !== createHash("sha256").update(readFileSync(source)).digest("hex")) {
    throw new Error("Corresponding FFmpeg source checksum mismatch");
  }
  for (const file of [host, join(native, "CaptureProbe.exe"), join(web, "index.html"), source,
    join(packageDir, "licenses/ffmpeg/COPYING.LGPLv2.1"), join(packageDir, "licenses/ffmpeg/NOTICE.txt")]) {
    if (!existsSync(file)) throw new Error(`Missing distribution input: ${file}`);
  }
  const expected = readFileSync(join(packageDir, "ffmpeg.sha256"), "utf8").trim().match(/^([a-f0-9]{64})\s+ffmpeg\.exe$/)?.[1];
  const actual = createHash("sha256").update(readFileSync(join(packageDir, "ffmpeg.exe"))).digest("hex");
  if (!expected || actual !== expected) throw new Error("Bundled FFmpeg checksum mismatch");
  // This is a generated directory, never an installation or user data directory.
  rmSync(output, { recursive: true, force: true });
  mkdirSync(join(output, "scripts"), { recursive: true });
  cpSync(host, join(output, "share-host.exe"));
  cpSync(native, join(output, "CaptureProbe"), { recursive: true });
  cpSync(web, join(output, "web"), { recursive: true });
  cpSync(packageDir, output, { recursive: true });
  cpSync(join(root, "scripts/create-rdp-credentials.ps1"), join(output, "scripts/create-rdp-credentials.ps1"));
  cpSync(join(root, "LICENSE"), join(output, "LICENSE"));
  return output;
}

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

function build(checkOnly) {
  let staging;
  try {
    const media = join(dist, "ffmpeg/package");
    if (!existsSync(join(media, "ffmpeg.exe"))) throw new Error("Build the bundled FFmpeg first: bash scripts/ffmpeg/build.sh (on Ubuntu / WSL), or download the share-app-ffmpeg Actions artifact into dist/ffmpeg.");
    if (isWindows) {
      const exe = join(media, "ffmpeg.exe");
      const license = inspect(exe, ["-hide_banner", "-L"]);
      const configuration = inspect(exe, ["-hide_banner", "-buildconf"]);
      if (!/GNU\s+Lesser\s+General\s+Public\s+License/.test(license) || /--enable-(gpl|nonfree|version3)/.test(configuration)) {
        throw new Error("Bundled FFmpeg must use the LGPL configuration.");
      }
      for (const flag of ["--disable-autodetect", "--disable-gpl", "--disable-nonfree", "--enable-libvpx"]) {
        if (!configuration.includes(flag)) throw new Error(`Missing FFmpeg configure flag: ${flag}`);
      }
      const pathKey = Object.keys(env).find((key) => key.toLowerCase() === "path") ?? "PATH";
      env[pathKey] = `${media};${env[pathKey] ?? ""}`;
    }
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
    run("node", ["--test", "scripts/test/distribution.test.mjs"]);

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

    run("go", ["build", "-ldflags=-H=windowsgui", "-o", host, "./cmd/share-host"], {
      cwd: server,
      env: { ...env, GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" },
    });
    run("go", ["vet", "./..."], { cwd: server });
    run("go", ["test", "-vet=off", "-count=1", "./..."], { cwd: server });

    if (!checkOnly) {
      stageDistribution({ host, native, web: join(web, "dist"), output });
      console.log(`\nWindows x64 distribution ready: ${output}`);
      console.log("Create Setup.exe and ZIP with installer/build.ps1.");
    }
  } catch (error) {
    console.error(`\nBuild failed: ${error.message}`);
    process.exitCode = 1;
  } finally {
    if (staging) rmSync(staging, { recursive: true, force: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  if (args.length > 1 || (args.length === 1 && args[0] !== "--check")) {
    console.error("Usage: node scripts/build.mjs [--check]");
    process.exitCode = 1;
  } else {
    build(args[0] === "--check");
  }
}

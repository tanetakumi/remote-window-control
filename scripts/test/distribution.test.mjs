import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { stageDistribution } from "../build.mjs";

function fixture(t) {
  const base = mkdtempSync(join(tmpdir(), "share-app-staging-"));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  function write(path, data = "fixture") {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, data);
  }
  const options = { host: join(base, "host.exe"), native: join(base, "native"), web: join(base, "web"), ffmpeg: join(base, "ffmpeg"), output: join(base, "output") };
  const binary = join(options.ffmpeg, "package/ffmpeg.exe");
  const version = readFileSync(new URL("../ffmpeg/versions.env", import.meta.url), "utf8").match(/^FFMPEG_VERSION=(\S+)$/m)[1];
  const sourceName = `ffmpeg-${version}-share-app-sources.tar.gz`;
  const source = join(options.ffmpeg, sourceName);
  for (const path of [options.host, join(options.native, "CaptureProbe.exe"), join(options.web, "index.html"), binary, source,
    join(options.ffmpeg, "package/licenses/ffmpeg/COPYING.LGPLv2.1"), join(options.ffmpeg, "package/licenses/ffmpeg/NOTICE.txt")]) write(path);
  const digest = createHash("sha256").update("fixture").digest("hex");
  write(join(options.ffmpeg, "package/ffmpeg.sha256"), `${digest}  ffmpeg.exe\n`);
  write(join(options.ffmpeg, "sources.sha256"), `${digest}  ${sourceName}\n`);
  write(join(options.output, "config.json"), "private stale build data");
  write(join(options.output, "web/old.js"), "stale web bundle");
  return { options, binary, source };
}

test("distribution replaces stale output and includes FFmpeg with notices", (t) => {
  const { options } = fixture(t);
  stageDistribution(options);
  for (const file of ["share-host.exe", "ffmpeg.exe", "CaptureProbe/CaptureProbe.exe", "web/index.html", "LICENSE", "licenses/ffmpeg/NOTICE.txt", "scripts/create-rdp-credentials.ps1"]) {
    assert.ok(existsSync(join(options.output, file)), file);
  }
  assert.equal(existsSync(join(options.output, "config.json")), false);
  assert.equal(existsSync(join(options.output, "web/old.js")), false);
});

test("a changed FFmpeg binary fails before replacing the previous output", (t) => {
  const { options, binary } = fixture(t);
  writeFileSync(binary, "unverified replacement");
  assert.throws(() => stageDistribution(options), /FFmpeg checksum mismatch/);
  assert.equal(readFileSync(join(options.output, "config.json"), "utf8"), "private stale build data");
});

test("missing or changed corresponding sources prevent distribution", (t) => {
  const { options, source } = fixture(t);
  writeFileSync(source, "wrong corresponding sources");
  assert.throws(() => stageDistribution(options), /source checksum mismatch/);
  rmSync(source);
  assert.throws(() => stageDistribution(options), /ENOENT/);
  assert.ok(existsSync(join(options.output, "web/old.js")));
});

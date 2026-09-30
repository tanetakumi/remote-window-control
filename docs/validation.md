# Validation record

Date: 2026-10-01. Environment: Linux/WSL2 x86_64, Intel Core i5-13500. No interactive Windows desktop or iPhone was available. No Windows/device or clean-machine acceptance result is claimed.

## Toolchains

Verified executables installed outside the repository, using official download metadata and checksum verification:

- Go `/tmp/rwc-toolchains/go/bin/go`: `go1.27.1 linux/amd64`
- .NET `/tmp/rwc-toolchains/dotnet/dotnet`: SDK `10.0.401`
- Node `/tmp/rwc-toolchains/node-v24.21.0-linux-x64/bin/node`: `v24.21.0`

These temporary paths record this session only; Windows scripts and CI resolve and verify their own installations.

## Passed automated checks

The initial client dependency install and Vite 5 build passed before modernization. The updated runtime was built in the required web UI → window capture → control server order.

| Check | Result |
| --- | --- |
| Client `npm install`, production build on Vite 8.3.1 | passed; icons generated |
| Client `npm test` | 6 passed, no skips |
| npm dependency audit | zero reported vulnerabilities after sharp 0.35.5 |
| .NET 10 Windows-targeted build | passed, zero warnings/errors |
| NuGet locked restore | passed |
| .NET Release build and framework-dependent win-x64 publish | passed; Windows apphost generated |
| NuGet vulnerability listing, including transitive dependencies | zero reported findings using current NuGet sources |
| Go Windows amd64 build and vet | passed |
| Portable Go package tests with `-race` | passed |
| Actual ffmpeg + synthetic capture helper lifecycle integration | passed, three stream/encoder shutdown cycles |
| Pending frame read cancellation | passed; stalled helper and reader terminated |
| Concurrent one-slot acquisition, second WebSocket rejection, reconnect after peer cleanup | passed |
| Shell syntax and diff whitespace checks | passed |

Client regressions cover two-finger touchend suppression, touchcancel button release, space/IME/replacement handling, bounded Unicode paste, connected/decoded-video readiness, early ICE, host errors, abort and listener disposal.

Go regressions cover authenticated loopback APIs, cross-origin rejection, secret/parent/traversal/symlink containment, snapshot path/device-name rejection, literal configuration and environment precedence, late input/held-input release, malformed/truncated/duplicate frames, reusable-buffer ownership, concurrent subprocess close, bounded concurrent diagnostics, slot ownership and real encoder cleanup.

Portable Go test command, from `control-server/`:

```bash
/path/to/verified/go test -race \
  ./internal/auth ./internal/config ./internal/httpserver ./internal/input \
  ./internal/nativecapture ./internal/processio ./internal/signaling \
  ./internal/targetwindow ./internal/webrtc ./internal/websecurity
```

The Win32 injector and application entry point are cross-built/vetted rather than executed on Linux. The encoder integration uses a synthetic Go subprocess in place of WGC. A passing synthetic test does not validate Windows capture or injection.

Native commands from the repository root:

```bash
/path/to/dotnet restore window-capture/apps/CaptureProbe/CaptureProbe.csproj --locked-mode -p:EnableWindowsTargeting=true
/path/to/dotnet build window-capture/apps/CaptureProbe/CaptureProbe.csproj -p:EnableWindowsTargeting=true
EnableWindowsTargeting=true /path/to/dotnet package list --project window-capture/apps/CaptureProbe/CaptureProbe.csproj --vulnerable --include-transitive
```

The environment property is required for package listing on Linux because the SDK performs restoration. Merely passing `--no-restore` to the legacy listing command failed in this SDK/environment; the command above succeeded. NuGet and npm audits reflect their current advisory sources, not a comprehensive security assessment.

## Allocation benchmark

Command from `control-server/`:

```bash
/path/to/verified/go test ./internal/nativecapture -run '^$' -bench ReadFrameInto -benchmem
```

1920×1080 BGRA, current validated reader, synthetic repeating input:

| Payload ownership | Time/read | Bytes allocated/read | Allocations/read |
| --- | ---: | ---: | ---: |
| Allocate each read | 871,890 ns | 8,298,520 | 1 |
| Reuse caller buffer | 239,104 ns | 0 | 0 |

Buffer setup is excluded from the timed section. This compares the two ownership modes in isolation, rather than running the complete old and new applications. It supports the allocation reduction claim only. It does not establish Windows CPU, WGC overhead, encoder throughput, network latency, quality or battery improvements.

Native per-frame Task.Run removal, copied-frame ID tracking and snapshot-copy removal compile, but have not been measured on Windows. Encoder settings remain at the original quality/10fps baseline.

## Interactive Windows gate

On an interactive Windows 11 machine:

1. Run `scripts/build.ps1 -Check`; record exact Windows build, GPU/driver, SDK versions and ffmpeg build/libvpx support. Exercise the CI workflow on the retained Windows runner.
2. Start from the repository root with `.env`, then from the packaged folder with a different current directory. Verify environment overrides, default HTTP :8443 binding and release-relative assets/helper/encoder lookup.
3. Authenticate through the mobile client and host UI. Confirm no loopback privilege bypass, no unauthenticated config/window/target/snapshot access and no unwanted static file access, including NTFS reparse points.
4. List/select a real window; stream, resize, minimize, restore, close and move it between virtual desktops. Switch to another selectable window while capture is stalled. Verify the old process exits before the new stream starts.
5. Open two tabs using the same saved token and two devices using different tokens. Confirm only one streaming capture/encoder pair exists; disconnect and reconnect repeatedly. Record process IDs and verify all exit after cleanup/server shutdown.
6. Test clicks at known client-area landmarks at 100%, 125%, 150% and mixed-monitor DPI; separately record behavior of non-web-ui/title-bar edges. Check right drag flags, scroll routing, long press and release on cancellation/target switch/disconnect. Test unresponsive target text/scroll/resize without blocking recovery.
7. Check PNG download, new named snapshot save, returned dimensions, collision rejection, arbitrary path rejection and CLI snapshots.
8. Measure actual fps/resolution, memory, Go allocations, per-process CPU, capture/encode/input latency and screenshot quality over identical workloads. Include a slow encoder and long-running session. Record queue bounds and process counts. Keep quality settings until the comparison supports tuning.

Record failures and fix them before marking these acceptance items complete. Background Win32 message injection varies by target application; cross-compilation cannot establish its behavior.

## iPhone/Safari gate

Use a separately supplied reachable endpoint; record iOS/Safari version, device, page origin and network/media path.

- First decoded video, muted autoplay and tap-to-play recovery.
- English UI/errors/accessibility, usable retry and window selection.
- Single/two-finger scroll, partial touchend, right drag, touchcancel and last-tap coordinates.
- Japanese composition/commit, English, spaces, paste/replacement, emoji, Backspace and Enter. Confirm no duplicate key/text paths and synchronous keyboard focus from a gesture.
- Keyboard viewport suppression, orientation changes, fullscreen availability and standalone PWA install guidance.
- Page background/foreground, pagehide/BFCache, connectivity loss, reload and host restart with stored-secret token refresh.

HTML fullscreen availability and secure-origin requirements vary by browser context. The UI reports unsupported fullscreen; standalone PWA mode remains available where supported. Endpoint/tunnel/relay provisioning is outside this repository's deliverables.

## Release gate

The current artifact is framework-dependent and requires external ffmpeg. After Windows validation and pipeline measurements, publish self-contained .NET 10 for win-x64, bundle a pinned ffmpeg/libvpx artifact with checksum and notices, and test on a clean Windows 11 desktop without development tools or separately installed runtime/encoder. Do not mark the distribution self-contained before that check.

## Repository consolidation revalidation

The source roots were renamed to `web-ui/`, `control-server/` and `window-capture/`. Development guidance is consolidated in README.md, the roadmap is in docs/roadmap.md, and Renovate configuration is in .github/renovate.json.

After relocation, the updated `scripts/build.sh` completed the web UI → window capture → control server sequence with the pinned SDKs. The six UI tests, Windows-targeted Go build/vet, portable Go race tests and NuGet locked restore passed again. Configuration regression coverage checks default discovery of `web-ui/dist/` from the renamed repository layout. Documentation links, solution project paths, JSON, shell syntax and ignored build artifacts were checked before committing. Windows/iPhone and release acceptance gates remain open.

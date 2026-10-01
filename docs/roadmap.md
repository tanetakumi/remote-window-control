# Windows 11 modernization plan

Updated 2026-10-01. Implementation is in progress. Portable regression tests and cross-builds have passed; interactive Windows, iPhone and clean-machine release checks remain open. Detailed evidence is in [validation.md](validation.md).

## Agreed product scope

- Native Windows 11 host; default HTTP listen address `127.0.0.1:8443` on loopback only.
- HTTP assets/APIs, WebSocket signaling, Pion WebRTC and existing Win32 input. Authentication belongs to the external Cloudflare Access layer; the application has no authentication code.
- Exactly one active control connection, including while the previous connection's workers are closing.
- iPhone browser client, initially Safari; no dedicated phone application.
- Existing window selection, WGC capture, gestures, keyboard/text, fullscreen, viewport resizing, bitrate, waiting display, host UI and snapshots remain.
- Long-lived CaptureProbe and ffmpeg processes; BGRA local pipes and VP8/IVF remain.
- Vendor-specific network integration is removed. External HTTPS, tunnels, NAT/relay provisioning, Docker and Cloudflare are outside implementation scope.
- Measure before tuning quality or claiming whole-application performance improvements. Self-contained delivery follows Windows optimization/compatibility validation.

## Structure and naming decisions

```text
web-ui/                                  mobile web client and test/
control-server/                          Go service and package-local tests
window-capture/
  apps/CaptureProbe/                      runtime executable
  src/WindowCapture.Native/              WGC library; Models.cs
scripts/                                 build/run and runner setup
docs/validation.md                       repeatable verification and remaining gates
```

Completed: moved CaptureProbe from `tests/` to `apps/`, moved root scripts into `scripts/`, renamed `Class1.cs` to `Models.cs`, and separated Windows input implementations with `_windows.go` filenames. Updated solution, bridge resolution, build scripts, CI and documentation together.

The source directories now describe their functions: `web-ui/` executes in the browser, `control-server/` serves HTTP/WebRTC and injects input, and `window-capture/` supplies WGC/list/snapshot. Updated asset/helper discovery, tests, scripts, CI and documentation together. Keep `share-host.exe`, `CaptureProbe.exe`, module/package names and supported environment names stable. `SHARE_APP_SECRET` was removed with application authentication. Do not introduce a new framework or merge projects without a demonstrated benefit.

Development guidance formerly in the root agent file is consolidated into README.md. Keep required root SDK/config/license files; the roadmap lives under docs/ and Renovate configuration under .github/.

## Selected toolchains and dependencies

Versions were checked against official release/registry metadata and installed with verified download checksums in the Linux development environment.

| Component | Selected version | State |
| --- | --- | --- |
| Go | 1.27.1 | Windows build/vet; portable tests with race detector |
| Node | 24.21.0 | production build and regression tests |
| Vite | 8.3.1 | upgraded from Vite 5; asset build verified |
| sharp | 0.35.5 | icon generation verified; npm audit zero findings |
| .NET SDK | 10.0.401 | .NET 10 Windows targeting and win-x64 apphost built |
| CsWinRT | 2.3.1 | use SDK-provided Windows projections; disable unnecessary custom projection generation |
| Vortice.Direct3D11 | 3.8.3 | updated RowPitch conversions; cross-build passed |
| ImageSharp | 3.1.12 | retain latest stable 3.x; 4.x introduces a build-time license-key requirement requiring a separate upgrade decision |
| Pion WebRTC | 4.2.22 | associated Pion modules upgraded; build/vet and lifecycle tests passed |
| Gorilla WebSocket | 1.5.3 | current stable retained |
| Go x/crypto, x/net, x/sys | 0.57.0, 0.59.0, 0.48.0 | compatible stable versions built and tested |

Vortice 3.8.3 retains its upstream SharpGen.Runtime/COM 2.4.2-beta dependencies in the locks; overriding those independently requires interop validation.

SDK versions are pinned by `.node-version`, `global.json` and `go.mod`. npm, Go and NuGet lock files are checked in. CI explicitly installs the selected toolchains and retains the self-hosted Windows runner and artifact/tag release behavior.

## Implemented changes and acceptance state

| Area | Implemented | Verification still needed |
| --- | --- | --- |
| Static serving | explicit single root, os.Root containment, hidden/directory/unsupported-type rejection, no parent fallback | Windows reparse-point cases |
| External access boundary | application authentication removed; loopback HTTP default; same-host Origin checks retained; target selection is shared without identity tracking | Cloudflare Access/Tunnel deployment and interactive host UI/device workflow |
| Snapshots | PNG download or new filename under snapshots/, exclusive creation, reserved-name/path rejection, one helper and bounded execution | real WGC PNG and file creation on Windows |
| Connection ownership | atomic slot acquired before peer creation, second connection rejected, shutdown waits for peer/process/input cleanup | repeated real-device connect/disconnect |
| Browser lifecycle | await media connection and decoded video, bounded readiness timeout, serialized answer/ICE, early ICE queue, explicit close, monitor/listener cleanup, return to selector, cancel pending connection on background/page exit | Safari autoplay/background/BFCache behavior |
| Input lifecycle | release held keys/buttons on cancel/disconnect/target change, serialized host input, reject late commands, bounded queues/messages | real unresponsive-window recovery |
| Gestures | suppress tap for the entire multi-touch gesture, release right hold on cancel, preserve scrolling and drag | iPhone touch behavior and rotation |
| Keyboard | text and space through input events, committed IME only, replacement suffix repair, special keys separated, direct gesture focus and cleanup | Japanese IME/paste/autocorrect across target applications |
| Windows input | map capture DWM physical bounds into client coordinates, process DPI awareness, drag button flags, BOOL result handling, bounded SendMessageTimeout, asynchronous window resizing | mixed DPI, non-client edges, application-specific routing |
| Capture lifecycle | change notifications cancel old helper without waiting for a frame; close is idempotent; native closed-window error; bounded helper execution | minimized/inactive-desktop/closed/resized WGC windows |
| Streaming | paced 10fps, one latest queued frame, three explicitly owned reusable buffers, validated dimensions/payload/IDs, synchronized bounded diagnostics, RTCP reader, process cancellation/reaping | actual fps/latency/CPU/quality and encoder stress on Windows |
| Native work | remove per-frame Task.Run, synchronous stream writes, track the copied frame ID, remove snapshot ToArray copy, serialize callback/disposal access | Windows interop/capture stress |
| Product/setup | retained strings are English; vendor integration removed; executable reads literal .env with environment overrides | Windows startup and external HTTPS origin workflow |
| Development CI | corrected helper location and win-x64 path; pinned setup, locked restore, tests/vet and development artifact packaging | execute workflow on Windows runner |

Tests demonstrate portable behavior, not Windows API or iPhone compatibility. Code changes that reduce native work have not yet been measured on an interactive desktop.

## Measurements

The synthetic 1920×1080 BGRA read benchmark compares allocating a payload on each read against caller-owned buffer reuse in the current validated reader. One Linux run measured approximately 8.3 MB and one allocation per read versus zero bytes/allocations after setup; read time was 0.872 ms versus 0.239 ms. This isolates allocation/copy work and does not measure WGC, encoding, network latency or visual quality. Re-run with `go test ./internal/nativecapture -run '^$' -bench ReadFrameInto -benchmem`.

Retain the existing encoder baseline: 10fps, libvpx/VP8, 6M bitrate, CRF 10, realtime deadline and cpu-used 4. Do not tune it from the synthetic benchmark.

## Revised remaining sequence

1. **Interactive Windows compatibility gate.** Run the checks in docs/validation.md on Windows 11. Validate .NET 10/WinRT/Vortice, list/stream/snapshot, resize, closure, stalls, held-input release and mixed-DPI coordinates. Fix failures before further optimization or distribution claims.
2. **Measure the full pipeline.** Record fps, resolution, capture/encode/input latency, CPU, memory, allocations and quality for the same windows and workloads. Include repeated target changes, stalled capture, slow encoder and connect/disconnect. Compare with the prior revision where it can run safely on an isolated network. Keep the baseline settings; tune only from measurements.
3. **Safari/iPhone gate.** Record iOS/browser version and a reachable test URL. Check English UI, actual playback, Japanese IME, paste/replacement, spaces/Backspace/Enter, drag/touchcancel, scroll, rotation, fullscreen support, keyboard/viewport and background/foreground recovery. External endpoint provisioning is a separate activity.
4. **Self-contained packaging.** After the compatibility/measurement gates, publish CaptureProbe for win-x64 with validated .NET 10 self-contained runtime. Bundle a pinned ffmpeg build with libvpx, checksum and redistribution notices. Use the existing release-local ffmpeg resolver and web/ root. Avoid trimming/AOT/single-file experiments until ordinary publishing works.
5. **Clean-machine and CI release gate.** Run the unzipped folder on a clean interactive Windows 11 machine with no Go/Node/.NET runtime/SDK/ffmpeg installation. Confirm config, external access, media, input and snapshots. Only then mark the distribution self-contained and update release claims. Continue shipping bundled runtime security patches in later releases.

Current CI packaging remains framework-dependent and uses external ffmpeg. This is explicitly documented as a development distribution. Cross-compilation does not satisfy the Windows, iPhone or clean-machine gates.

## Completion criteria

- Windows 11 serves HTTP on `127.0.0.1:8443` by default; external authentication is enforced by Cloudflare Access/Tunnel, with no application login or credential store.
- Only client assets are exposed; caller snapshot paths cannot write outside the designated directory.
- Exactly one control connection and at most one streaming capture/encoder pair; separate snapshots remain bounded.
- Retained features work with English product text and the existing WGC/Win32 approach.
- Stalled/closed windows do not prevent target changes; retries and Japanese IME work correctly.
- Repeated lifecycle tests leave no orphan processes, held input, accumulated queues, timers or connection slots.
- Full-pipeline measurements substantiate claimed performance improvements.
- iPhone compatibility and clean Windows self-contained execution are explicitly checked.
- No vendor deployment, tunnel or relay implementation is introduced.

## Primary references

- [Go releases](https://go.dev/dl/) and [support policy](https://go.dev/doc/devel/release)
- [Node releases](https://nodejs.org/en/about/previous-releases)
- [Vite releases](https://vite.dev/releases)
- [.NET 10 downloads](https://dotnet.microsoft.com/en-us/download/dotnet/10.0) and [support policy](https://dotnet.microsoft.com/en-us/platform/support/policy/dotnet-core)
- [CsWinRT package options](https://github.com/microsoft/CsWinRT/blob/master/nuget/readme.md)
- [Vortice changelog](https://github.com/amerkoleci/Vortice.Windows/blob/main/CHANGELOG.md)
- [ImageSharp 4 changes](https://sixlabors.com/posts/announcing-imagesharp-400/)
- [SendMessageTimeout](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessagetimeoutw)
- [.NET self-contained publication](https://learn.microsoft.com/en-us/dotnet/core/deploying/)
- [FFmpeg downloads](https://ffmpeg.org/download.html)

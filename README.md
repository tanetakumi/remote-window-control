# Share App

Control a selected Windows 11 application window from a mobile browser. The Windows host serves a vanilla JavaScript web client, streams WGC video through ffmpeg and Pion WebRTC, and receives touch and keyboard input through a WebRTC DataChannel; the WebSocket carries only signaling. One control connection is allowed at a time.

## Source layout

```text
web-ui/                                Vite mobile web client; regression tests in test/
control-server/                        Go host (module share-app-host)
  cmd/share-host/                      entry point
  internal/
    app/                               wiring, lifecycle, target-switching service
    config/                            .env / environment settings and path discovery
    httpapi/                           JSON API, static client, host page, same-origin guard
    session/                           the single control WebSocket: signaling and input
    media/                             WebRTC peer and the capture -> ffmpeg VP8 pipeline
    capture/                           CaptureProbe client: window list, snapshots, frame stream
    window/                            window model and the shared target selection
    input/                             input commands, dispatcher and window-message injector
    win32/                             Win32 calls (stubbed off Windows) and portable layout rules
    origin/                            same-origin policy
    tailbuf/                           bounded tail of subprocess diagnostics
    hostlog/                           persistent host log with size-based rotation
  test/                                all Go tests, one directory per package; testutil/ is shared
window-capture/
  apps/CaptureProbe/                    single project: window list, WGC stream and PNG snapshots
```

`CaptureProbe` lives under `apps/` and contains five source files: CLI/output, window enumeration and its model, WGC capture and frame metadata, graphics interop/initialization, and PNG snapshots. Frame copies grow the caller's reusable buffer and copy pixels under one lock, keeping pixels and metadata consistent during resizing. Go tests live under `control-server/test/`, one directory per package under test, and use only exported APIs. Operating-system calls are isolated in `internal/win32` (`*_windows.go`, with stubs for other platforms), so every Go package builds, vets and is tested on Linux; only the Win32 calls themselves need Windows. Executable names remain stable; obsolete authentication APIs/settings and network-vendor settings have been removed.

The web UI is served by the Windows machine but executes in the phone browser. The control server handles signaling and input, while window capture provides Windows-specific frame capture. Authentication is managed externally by Cloudflare Access. The directories are named for these functions. The executable names remain stable.

## Repository files

- `.gitignore` excludes generated artifacts, local secrets, certificates and snapshots.
- `.env.example` documents executable configuration; `LICENSE` contains the project license.
- `.github/distribution/はじめにお読みください.txt` is copied into the root of the Windows distribution ZIP.
- `.node-version` and `global.json` pin SDKs and are consumed by CI and .NET respectively; Go is pinned in `control-server/go.mod`.
- `.github/renovate.json` retains dependency update configuration; workflow definitions also live under `.github/`.

## Build

GitHub Actions builds and tests the application and prepares the Windows distribution. Use the Actions artifact or GitHub Release ZIP for installation; local build and runner setup scripts are not needed.

Install the versions pinned in `.node-version`, `global.json` and `control-server/go.mod`: Node 24.21.0, .NET SDK 10.0.401 and Go 1.27.1. Streaming currently also requires an ffmpeg build with `libvpx` on PATH, and the .NET 10 runtime for the framework-dependent helper.

For local development on Windows, run the following commands from PowerShell in the repository root. The required build order is web UI → window capture → control server:

```powershell
cd web-ui
npm ci
npm run build
npm test
cd ..
dotnet restore window-capture/apps/CaptureProbe/CaptureProbe.csproj --locked-mode
dotnet build window-capture/apps/CaptureProbe/CaptureProbe.csproj --no-restore
cd control-server
$goExe = (Get-Command go).Source
& $goExe version # Verify Go 1.27.1 before continuing.
& $goExe build -o bin/share-host.exe ./cmd/share-host
& $goExe vet ./...
& $goExe test ./...
```

Go installation is explicit in CI, with pinned Node/.NET/Go setup and locked dependency restoration. Pull requests, default-branch pushes, tag pushes and manual runs build and test on a GitHub-hosted Windows runner; pushes and manual runs also publish/package the distribution. No self-hosted runner is required. Only the separate tag release job has repository write permission. New updates cancel older runs for the same pull request. CI installs ffmpeg 8.1.1 through Chocolatey and verifies `libvpx` encoder support before running the Go tests, without reusing cached test results. Dependency locks are checked in; build outputs are ignored.

Linux can build the client, cross-build CaptureProbe with `-p:EnableWindowsTargeting=true`, and cross-build the host with `GOOS=windows GOARCH=amd64`. From `control-server`, `go vet ./...` and `go test ./...` also run on Linux (the encoder integration test needs ffmpeg with `libvpx` and is skipped without it). Linux cannot execute WGC or Win32 input.

## Run and configuration

For local development after building from source, optionally copy `.env.example` to `.env` in the repository root to override configuration. Run from the repository root:

```powershell
.\control-server\bin\share-host.exe
```

When using an extracted distribution ZIP, install the .NET 10 x64 runtime and provide FFmpeg with the `libvpx` encoder on `PATH` (or place `ffmpeg.exe` beside `share-host.exe`). The ZIP includes a `.env` beside `share-host.exe`, preconfigured with `SHARE_APP_ADDR=127.0.0.1:8443`; edit it if you need a different listen address, then run from the extracted folder. See `はじめにお読みください.txt` in the ZIP for Windows setup and LAN access:

```powershell
.\share-host.exe
```

The default is HTTP on `127.0.0.1:8443`, listening on loopback only. Open `http://127.0.0.1:8443/` on Windows, or the externally configured HTTPS URL on the phone. Select a window to start control. A minimized window is restored without activating it, because capture delivers no frames for it; the viewport size is sent as soon as the control channel opens. The connection is ready after WebRTC connects and the browser receives decoded video. Use the window button to disconnect and select another window. Backgrounding the page closes the connection; select a window again on return.

The host also writes its log to `logs/share-host.log` beside `share-host.exe`, whatever the working directory. It is appended across restarts, uses UTC timestamps, and rotates at 5 MiB keeping `.1` and `.2`. It records startup and configuration, window selection, each connection (WebRTC state, control channel, capture start/stop, first captured frame, first video sample, a warning when no frame arrives for five seconds) and errors, including CaptureProbe and ffmpeg stderr. Input, pixels and SDP are not logged. If the log cannot be opened, startup fails and says why.

The executable reads a literal `KEY=VALUE` `.env` file; it does not execute shell commands. Supported keys are:

| Key | Default | Meaning |
| --- | --- | --- |
| `SHARE_APP_ADDR` | `127.0.0.1:8443` | HTTP listen address; the port does not imply TLS |

Environment variables override the file. Single or double quotes around a whole value are optional; inline shell expansions and inline comments are not interpreted. The web client is found automatically: `web/` beside the executable in a release, `web-ui/dist/` in a checkout. Only `.html`, `.css` and `.js` files within that directory are served; parent directories, directory listings, hidden files and escaping symlinks are rejected.

The application has no login, shared secret, access tokens or authentication endpoint. The mobile UI loads immediately and uses the same APIs as any direct caller. Browser API and WebSocket requests must come from the same host as the page. One control connection is allowed at a time; target selection is shared across clients, including during an active connection.

For external access, run Cloudflare Tunnel on the Windows host and route the public hostname to `http://127.0.0.1:8443`. Protect the entire hostname with Cloudflare Access, including `/api/*` and `/ws`, and enable token validation in `cloudflared` using [Protect with Access](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/self-hosted-public-app/). Keep the origin reachable only through the trusted proxy for remote clients. Preserve the public HTTP `Host` header so the application's Origin checks continue to work. Local loopback access requires no authentication; Cloudflare-specific integration remains outside the application code.

When upgrading, remove `SHARE_APP_SECRET` and `SHARE_APP_CLIENT_DIR` from existing `.env` files; they are no longer supported configuration keys, and the host refuses to start with an error naming the offending line. Client-side credential storage and secret-bearing URLs are no longer used.

Client URLs follow the page origin, including HTTPS/WSS when HTTPS is supplied externally. HTTPS termination, domains, tunnels, NAT traversal and relay provisioning are managed separately. HTTP reachability does not establish WebRTC media reachability. Safari features requiring a secure origin should be tested through a separately supplied HTTPS URL.

## Input and snapshots

The remote video is touch-only, with two modes selected using the Input mode control. Relative pointer is the default: slide one finger to move the visible cursor, lift and reposition without moving it, and tap anywhere within the image to click at the cursor. Direct tap clicks the touched image position. The selected mode is saved locally when browser storage is available.

| Touch gesture | Relative pointer | Direct tap |
| --- | --- | --- |
| One-finger slide | Move the cursor without pressing a button | Scroll at the initial touch position |
| Tap | Left-click at the cursor | Left-click at the touched position |
| Double-tap, holding the second tap, then move | Left-button drag from the cursor | Left-button drag from the touched position |
| Long press (450 ms) | Right-click at the cursor | Right-click at the touched position |
| Slide after a long press | Continue moving the cursor without holding a button; lifting does not click again | No further action until the finger lifts |
| Two-finger tap | Right-click at the cursor | Right-click at the center of the touches |
| Two-finger slide | Scroll at the cursor | Scroll at the center where the gesture began |

Physical mouse hover, buttons and wheel input are not forwarded. Pinches are suppressed rather than sent as scrolling; video zoom is not implemented. Scrolling is vertical, uses the same direction for one and two fingers, and converts 100 CSS pixels into one wheel notch while accumulating smaller movements. New gestures must begin inside the displayed image; letterbox margins cannot click the window. Logical cursor coordinates are preserved across mode changes and video resizing, and start at the center for each new connection. The visible cursor is an overlay for the selected window, rather than the host's physical Windows cursor.

Text, paste, Japanese IME, Backspace, Enter, viewport resizing, fullscreen where supported, and bitrate display are retained. Composing text stays local until committed. Held input is released on gesture cancellation, blur, page hide, mode changes, disconnect and target changes. After a gesture is interrupted, remaining contacts are ignored until all fingers lift. Win32 text and scroll calls have bounded waits.

Call the local APIs directly for debugging:

```powershell
Invoke-RestMethod 'http://127.0.0.1:8443/api/windows'
Invoke-RestMethod 'http://127.0.0.1:8443/api/target-window' -Method Post -ContentType 'application/json' -Body '{"handle":657830}'
```

`GET /api/snapshot` returns PNG bytes for the selected target. The host never writes snapshots to disk and cannot capture a window other than the selected one; the `out` and `hwnd` parameters are refused. One snapshot helper runs at a time, with a 10-second execution timeout. To keep a copy, save the response:

```powershell
Invoke-WebRequest 'http://127.0.0.1:8443/api/snapshot' -OutFile local-copy.png
```

Streaming uses a single long-lived capture process and encoder, never one process per frame. The newest captured frame is re-encoded at the configured rate even when the window sends no new one, so a static window keeps producing video and periodic keyframes.

## Current distribution status

Pushes to `master` or `main` and manual runs produce the `share-app-windows` Actions artifact. Downloading it gives a ZIP with `share-host.exe` and the supporting files directly at its root; there is no ZIP inside it. Tag pushes matching `v*` also upload this artifact, then the release job packages its contents as `share-app-<tag>.zip` and attaches it to the GitHub Release after the build and tests pass. Both downloads need only one extraction. Pull requests run build/test checks without publishing or uploading a distribution. To build manually, open Actions → Build → Run workflow and select `master`.

To try an Actions build on Windows 11 x64:

1. Open [Actions → Build](https://github.com/tanetakumi/remote-window-control/actions/workflows/build.yml), select a successful `master` run, and download `share-app-windows` under Artifacts (sign in to GitHub if needed).
2. Extract the downloaded ZIP once to a folder. `share-host.exe` is directly inside that folder.
3. Install the Windows x64 [.NET 10 Runtime](https://dotnet.microsoft.com/en-us/download/dotnet/10.0) and provide FFmpeg with `libvpx` on `PATH` (or place `ffmpeg.exe` beside `share-host.exe`).
4. Run `.\share-host.exe` from that folder in PowerShell. Open `http://127.0.0.1:8443/` on the same Windows machine and select a window.

For phone access, configure Cloudflare Tunnel and Access as described under Run and configuration, then open the protected HTTPS hostname on the phone. The loopback URL above is for the Windows host only.

The ZIP contains `share-host.exe`, `CaptureProbe/`, `web/`, `.env`, `はじめにお読みください.txt` and LICENSE. The `.env` sets `SHARE_APP_ADDR=127.0.0.1:8443`. It remains a **development distribution**: the .NET 10 runtime and FFmpeg are external requirements. The host resolves `ffmpeg.exe` beside its executable or on `PATH`.

Self-contained publishing, a pinned ffmpeg bundle with checksum/notices, and a clean Windows 11 installation test remain pending after Windows performance and device validation.

WGC can stop updating a minimized window or a window on an inactive virtual desktop. Target switching cancels the old helper independently of frame arrival. Actual WGC capture, mixed-DPI click alignment, application-specific background input and iPhone behavior still require an interactive Windows/device test.

## Development and maintenance

Runtime changes must build in the order web UI → window capture → control server. Verify the Go executable path and version before building. Keep npm, NuGet and Go dependency locks in version control, and rebuild generated outputs locally. Do not commit dependency folders, build/publish output, captures, secrets, certificates or keys. Retain existing user features and the WGC/Win32 architecture; retained product strings are English.

Streaming ownership rules:

- Acquire the single control-connection slot before creating a peer; release it after processes, callbacks and held input have closed.
- Keep one long-lived CaptureProbe per target and one long-lived ffmpeg encoder, restarted only when the frame size changes. Keep the newest frame's buffer until a newer one replaces it. Snapshot helpers are separate bounded operations. Never start streaming processes per frame.
- Validate the 24-byte BGRA header, bound the latest-frame handoff, and keep reusable-buffer ownership explicit.
- Target switching must cancel capture without waiting for another frame. Consume RTCP, bound and synchronize diagnostics, and keep shutdown idempotent.
- Preserve the 10fps/quality baseline until interactive Windows measurements justify tuning. Serve only explicit client asset roots; never restore parent-directory fallback. Keep authentication in the external access layer and bind HTTP to loopback by default.

Key implementation locations:

| Function | Source |
| --- | --- |
| Touch modes, gestures and release | `web-ui/src/input/gestures.js` |
| Touch mode selector, preference and cursor overlay | `web-ui/src/input/touch-ui.js` |
| Shared video coordinates | `web-ui/src/input/coordinates.js` |
| Committed text, IME and special keys | `web-ui/src/input/keyboard.js` |
| Connection readiness, ICE and cleanup | `web-ui/src/core/webrtc.js` |
| Window list and target selection APIs | `web-ui/src/core/api.js` |
| Input validation, held keys/buttons and release | `control-server/internal/input/dispatcher.go` |
| Window-message input, coordinates and bounded Win32 calls | `control-server/internal/input/message_injector.go`, `control-server/internal/win32/` |
| Capture helper location and settings paths | `control-server/internal/config/paths.go` |
| Capture helper commands and validated frame reads | `control-server/internal/capture/probe.go`, `stream.go`, `frame.go` |
| Capture/encoder ownership and pacing | `control-server/internal/media/pipeline.go`, `framepump.go`, `encoder.go` |
| Single-connection slot and WebSocket lifecycle | `control-server/internal/session/slot.go`, `hub.go` |
| Target switching with input release | `control-server/internal/app/targets.go` |
| HTTP API, static serving and origin guard | `control-server/internal/httpapi/` |
| Native stream protocol | `window-capture/apps/CaptureProbe/Program.cs` |
| WGC lifecycle and copying | `window-capture/apps/CaptureProbe/WgcCaptureService.cs` |

The control server resolves CaptureProbe from release `CaptureProbe/CaptureProbe.exe`, or development `window-capture/apps/CaptureProbe/bin/{Debug,Release}/net10.0-windows10.0.19041.0/win-x64/CaptureProbe.exe`. Verify Windows/iPhone compatibility, end-to-end performance gains and self-contained release readiness on the corresponding platforms before making those claims.

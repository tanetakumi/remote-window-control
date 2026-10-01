# Share App

Control a selected Windows 11 application window from a mobile browser. The Windows host serves a vanilla JavaScript web client, streams WGC video through ffmpeg and Pion WebRTC, and receives touch and keyboard input through a DataChannel or WebSocket. One control connection is allowed at a time.

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
  test/                                all Go tests, one directory per package; testutil/ is shared
window-capture/
  apps/CaptureProbe/                    window list, stream and snapshot CLI
  src/WindowCapture.Native/            WGC capture library
scripts/                               build, run and runner setup tools
```

`CaptureProbe` is a runtime application, so it now lives under `apps/`. `Models.cs` contains the capture models. Go tests live under `control-server/test/`, one directory per package under test, and use only exported APIs. Operating-system calls are isolated in `internal/win32` (`*_windows.go`, with stubs for other platforms), so every Go package builds, vets and is tested on Linux; only the Win32 calls themselves need Windows. Executable names remain stable; obsolete authentication APIs/settings and network-vendor settings have been removed.

The web UI is served by the Windows machine but executes in the phone browser. The control server handles signaling and input, while window capture provides Windows-specific frame capture. Authentication is managed externally by Cloudflare Access. The directories are named for these functions. The native library and executable names remain stable.

## Repository files

- `.gitignore` excludes generated artifacts, local secrets, certificates and snapshots.
- `.env.example` documents executable configuration; `LICENSE` contains the project license.
- `.node-version` and `global.json` pin SDKs and are consumed by build scripts/CI and .NET respectively; Go is pinned in `control-server/go.mod`.
- `.github/renovate.json` retains dependency update configuration; workflow definitions also live under `.github/`.

## Build on Windows

Install the versions pinned in `.node-version`, `global.json` and `control-server/go.mod`: Node 24.21.0, .NET SDK 10.0.401 and Go 1.27.1. Streaming currently also requires an ffmpeg build with `libvpx` on PATH, and the .NET 10 runtime for the framework-dependent helper.

From PowerShell in the repository root:

```powershell
./scripts/build.ps1 -Check
# To select a specific Go installation:
./scripts/build.ps1 -GoExe 'C:\Program Files\Go\bin\go.exe' -Check
```

The required build order is web UI → window capture → control server. Manual equivalents:

```powershell
cd web-ui
npm install
npm run build
npm test
cd ..
dotnet build window-capture/apps/CaptureProbe/CaptureProbe.csproj
cd control-server
$goExe = (Get-Command go).Source
& $goExe version # Verify Go 1.27.1 before continuing.
& $goExe build -o bin/share-host.exe ./cmd/share-host
& $goExe vet ./...
& $goExe test ./...
```

Go installation is explicit in CI, with pinned Node/.NET/Go setup and locked dependency restoration. Pull requests build and test on a GitHub-hosted Windows runner; branch and tag pushes use the retained self-hosted Windows runner and also publish/package the distribution. Only the separate tag release job has repository write permission. New updates cancel older runs for the same pull request. CI installs ffmpeg 8.1.1 through Chocolatey on the PR runner; the self-hosted runner must already provide ffmpeg on PATH. CI verifies `libvpx` encoder support before running the Go tests, without reusing cached test results. Dependency locks are checked in; build outputs are ignored.

Linux can build the client, cross-build CaptureProbe with `-p:EnableWindowsTargeting=true`, and cross-build the host with `GOOS=windows GOARCH=amd64`. From `control-server`, `go vet ./...` and `go test ./...` also run on Linux (the encoder integration test needs ffmpeg with `libvpx` and is skipped without it). Linux cannot execute WGC or Win32 input.

## Run and configuration

When running from source, optionally copy `.env.example` to `.env` in the repository root to override configuration. Run:

```powershell
./scripts/run.ps1
```

When using an extracted distribution ZIP, install the .NET 10 x64 runtime and provide ffmpeg with `libvpx` on PATH (or put `ffmpeg.exe` beside `share-host.exe`). Optionally copy `.env.example` to `.env` beside `share-host.exe`, then run from the extracted folder:

```powershell
.\share-host.exe
```

The default is HTTP on `127.0.0.1:8443`, listening on loopback only. Open `http://127.0.0.1:8443/` on Windows, or the externally configured HTTPS URL on the phone. Select a window to start control. The connection is ready after WebRTC connects and the browser receives decoded video. Use the window button to disconnect and select another window. Backgrounding the page closes the connection; select a window again on return.

The executable reads a literal `KEY=VALUE` `.env` file; it does not execute shell commands. Supported keys are:

| Key | Default | Meaning |
| --- | --- | --- |
| `SHARE_APP_ADDR` | `127.0.0.1:8443` | HTTP listen address; the port does not imply TLS |
| `SHARE_APP_CLIENT_DIR` | release `web/`, development `web-ui/dist/` | Explicit client asset directory |

Environment variables override the file. Single or double quotes around a whole value are optional; inline shell expansions and inline comments are not interpreted. Relative client paths resolve from the release/repository root. Only recognized client asset types within that directory are served; parent directories, directory listings, hidden files and escaping symlinks are rejected.

The application has no login, shared secret, access tokens or authentication endpoint. The mobile UI loads immediately and uses the same APIs as any direct caller. Browser API and WebSocket requests must come from the same host as the page. One control connection is allowed at a time; target selection is shared across clients, including during an active connection.

For external access, run Cloudflare Tunnel on the Windows host and route the public hostname to `http://127.0.0.1:8443`. Protect the entire hostname with Cloudflare Access, including `/api/*` and `/ws`, and enable token validation in `cloudflared` using [Protect with Access](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/self-hosted-public-app/). Keep the origin reachable only through the trusted proxy for remote clients. Preserve the public HTTP `Host` header so the application's Origin checks continue to work. Local loopback access requires no authentication; Cloudflare-specific integration remains outside the application code.

When upgrading, remove `SHARE_APP_SECRET` from existing `.env` files; it is no longer a supported configuration key. Client-side credential storage and secret-bearing URLs are no longer used.

Client URLs follow the page origin, including HTTPS/WSS when HTTPS is supplied externally. HTTPS termination, domains, tunnels, NAT traversal and relay provisioning are managed separately. HTTP reachability does not establish WebRTC media reachability. Safari features requiring a secure origin should be tested through a separately supplied HTTPS URL.

## Input and snapshots

Retained controls include tap, long press/right-button drag, one- and two-finger scrolling, text, paste, Japanese IME, Backspace, Enter, viewport resizing, fullscreen where the browser supports it and bitrate display. Scroll targets the last tap. Composing text stays local until committed. Held input is released on cancellation, disconnect and target changes. Win32 text and scroll calls have bounded waits.

Call the local APIs directly for debugging:

```powershell
Invoke-RestMethod 'http://127.0.0.1:8443/api/windows'
Invoke-RestMethod 'http://127.0.0.1:8443/api/target-window' -Method Post -ContentType 'application/json' -Body '{"handle":657830}'
```

`GET /api/snapshot` returns PNG bytes for the selected target. The host never writes snapshots to disk and cannot capture a window other than the selected one; the `out` and `hwnd` parameters are refused. One snapshot helper runs at a time, with a 10-second execution timeout.

To save a snapshot of any window to a file, use the CaptureProbe command line:

```powershell
Invoke-WebRequest 'http://127.0.0.1:8443/api/snapshot' -OutFile local-copy.png
dotnet run --project window-capture/apps/CaptureProbe/CaptureProbe.csproj -- --hwnd 657830 --out 'D:\captures\probe.png'
```

CLI snapshots use local caller-selected paths. Streaming uses a single long-lived capture process and encoder, never one process per frame.

## Current distribution status

Branch pushes produce the `share-app-windows` Actions artifact containing `share-app-<run-number>-<commit-sha>.zip`. Tag pushes matching `v*` produce `share-app-<tag>.zip`, upload it as an Actions artifact for transfer, and attach it to the GitHub Release after the build and tests pass. Pull requests run build/test checks without publishing or uploading a distribution.

The ZIP contains `share-host.exe`, `CaptureProbe/`, `web/`, `.env.example`, README.md and LICENSE. It remains a **development distribution**: .NET 10 runtime and ffmpeg are external requirements. The host also resolves `ffmpeg.exe` beside its executable when supplied.

Self-contained publishing, a pinned ffmpeg bundle with checksum/notices, and a clean Windows 11 installation test remain pending after Windows performance and device validation.

WGC can stop updating a minimized window or a window on an inactive virtual desktop. Target switching cancels the old helper independently of frame arrival. Actual WGC capture, mixed-DPI click alignment, application-specific background input and iPhone behavior still require an interactive Windows/device test.

## Development and maintenance

Runtime changes must build in the order web UI → window capture → control server. Verify the Go executable path and version before building. Keep npm, NuGet and Go dependency locks in version control, and rebuild generated outputs locally. Do not commit dependency folders, build/publish output, captures, secrets, certificates or keys. Retain existing user features and the WGC/Win32 architecture; retained product strings are English.

Streaming ownership rules:

- Acquire the single control-connection slot before creating a peer; release it after processes, callbacks and held input have closed.
- Keep one long-lived CaptureProbe per target and one long-lived ffmpeg encoder. Snapshot helpers are separate bounded operations. Never start streaming processes per frame.
- Validate the 24-byte BGRA header, bound the latest-frame handoff, and keep reusable-buffer ownership explicit.
- Target switching must cancel capture without waiting for another frame. Consume RTCP, bound and synchronize diagnostics, and keep shutdown idempotent.
- Preserve the 10fps/quality baseline until interactive Windows measurements justify tuning. Serve only explicit client asset roots; never restore parent-directory fallback. Keep authentication in the external access layer and bind HTTP to loopback by default.

Key implementation locations:

| Function | Source |
| --- | --- |
| Touch gestures and release | `web-ui/src/input/gestures.js` |
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
| WGC lifecycle and copying | `window-capture/src/WindowCapture.Native/WgcCaptureService.cs` |

The control server resolves CaptureProbe from release `CaptureProbe/CaptureProbe.exe`, or development `window-capture/apps/CaptureProbe/bin/{Debug,Release}/net10.0-windows10.0.19041.0/win-x64/CaptureProbe.exe`. Verify Windows/iPhone compatibility, end-to-end performance gains and self-contained release readiness on the corresponding platforms before making those claims.

# Share App

Control a selected Windows 11 application window from a mobile browser. The Windows host serves a vanilla JavaScript PWA, streams WGC video through ffmpeg and Pion WebRTC, and receives touch and keyboard input through a DataChannel or WebSocket. One control connection is allowed at a time.

## Source layout

```text
web-ui/                                Vite mobile PWA; regression tests in test/
control-server/                        Go server and package tests
window-capture/
  apps/CaptureProbe/                    window list, stream and snapshot CLI
  src/WindowCapture.Native/            WGC capture library
scripts/                               build, run and runner setup tools
docs/validation.md                     verification results and Windows/device checks
```

`CaptureProbe` is a runtime application, so it now lives under `apps/`. `Models.cs` contains the capture models. Windows input implementations use `_windows.go` filenames; the dispatcher, HTTP handlers and media process supervision can be tested on Linux. Executable names, API names and existing environment variable names are retained except for the removed network-vendor settings.

The web UI is served by the Windows machine but executes in the phone browser. The control server handles authentication, signaling and input, while window capture provides Windows-specific frame capture. The directories are named for these functions. The native library and executable names remain stable.

## Repository files

- `.gitignore` excludes generated artifacts, local secrets, certificates and snapshots.
- `.env.example` documents executable configuration; `LICENSE` contains the project license.
- `.node-version` and `global.json` pin SDKs and are consumed by build scripts/CI and .NET respectively; Go is pinned in `control-server/go.mod`.
- `.github/renovate.json` retains dependency update configuration; workflow definitions also live under `.github/`.
- `docs/roadmap.md` records implementation status and remaining work; `docs/validation.md` records verification evidence.

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
& $goExe build -o share-host.exe ./cmd/share-host
& $goExe vet ./...
& $goExe test ./...
```

Go installation is explicit in CI, with pinned Node/.NET/Go setup and locked dependency restoration. Pull requests build and test on a GitHub-hosted Windows runner; branch and tag pushes use the retained self-hosted Windows runner and also publish/package the distribution. Only the separate tag release job has repository write permission. New updates cancel older runs for the same pull request. CI installs ffmpeg 8.1.1 through Chocolatey on the PR runner; the self-hosted runner must already provide ffmpeg on PATH. CI verifies `libvpx` encoder support before running the Go tests, without reusing cached test results. Dependency locks are checked in; build outputs are ignored.

Linux can build the client, cross-build CaptureProbe with `-p:EnableWindowsTargeting=true`, and cross-build the host with `GOOS=windows GOARCH=amd64`. Linux cannot execute WGC or Win32 input. See [validation](docs/validation.md) for the tested commands and their limits.

## Run and configuration

When running from source, copy `.env.example` to `.env` in the repository root and set a long random `SHARE_APP_SECRET`. Run:

```powershell
./scripts/run.ps1
```

When using an extracted distribution ZIP, install the .NET 10 x64 runtime and provide ffmpeg with `libvpx` on PATH (or put `ffmpeg.exe` beside `share-host.exe`). Copy `.env.example` to `.env` beside `share-host.exe`, set `SHARE_APP_SECRET`, then run from the extracted folder:

```powershell
.\share-host.exe
```

The default is HTTP on `:8443`, listening on all interfaces. Open `http://<Windows-IP>:8443/?secret=<your-secret>` on the phone, or the logged local link on Windows. Select a window to start control. The connection is ready after WebRTC connects and the browser receives decoded video. Use the window button to disconnect and select another window. Backgrounding the page closes the connection; select a window again on return.

The executable reads a literal `KEY=VALUE` `.env` file; it does not execute shell commands. Supported keys are:

| Key | Default | Meaning |
| --- | --- | --- |
| `SHARE_APP_ADDR` | `:8443` | HTTP listen address; the port does not imply TLS |
| `SHARE_APP_SECRET` | random at startup | Shared secret used to obtain access tokens |
| `SHARE_APP_CLIENT_DIR` | release `web/`, development `web-ui/dist/` | Explicit client asset directory |

Environment variables override the file. Single or double quotes around a whole value are optional; inline shell expansions and inline comments are not interpreted. Relative client paths resolve from the release/repository root. Only recognized client asset types within that directory are served; parent directories, directory listings, hidden files and escaping symlinks are rejected.

HTTP APIs require a Bearer token even on loopback. Tokens expire after 12 hours; the client retries authentication using its saved secret. `/host-ui` serves a sign-in shell and uses the same authenticated APIs. Browser API and WebSocket requests must come from the same host as the page. The secret is removed from the client URL after authentication.

Client URLs follow the page origin, including HTTPS/WSS when HTTPS is supplied externally. HTTPS termination, domains, tunnels, NAT traversal and relay provisioning are managed separately. HTTP reachability does not establish WebRTC media reachability. Safari/PWA features requiring a secure origin should be tested through a separately supplied HTTPS URL.

## Input and snapshots

Retained controls include tap, long press/right-button drag, one- and two-finger scrolling, text, paste, Japanese IME, Backspace, Enter, viewport resizing, fullscreen where the browser supports it, bitrate display and PWA installation guidance. Scroll targets the last tap. Composing text stays local until committed. Held input is released on cancellation, disconnect and target changes. Win32 text and scroll calls have bounded waits.

Obtain a token for debugging:

```powershell
$session = Invoke-RestMethod 'http://127.0.0.1:8443/api/session' -Method Post -ContentType 'application/json' -Body '{"secret":"your-secret"}'
$headers = @{ Authorization = "Bearer $($session.access_token)" }
Invoke-RestMethod 'http://127.0.0.1:8443/api/windows' -Headers $headers
Invoke-RestMethod 'http://127.0.0.1:8443/api/target-window' -Method Post -Headers $headers -ContentType 'application/json' -Body '{"handle":657830}'
```

`GET /api/snapshot` returns PNG bytes for the selected target; optional `hwnd` selects a handle for that request. Optional `out=window.png` saves a **new simple PNG filename** within `snapshots/` beside the release/repository root and returns its relative path and dimensions. Existing files are not overwritten. Absolute paths, directories, traversal and Windows device names are rejected. One snapshot helper runs at a time, with a 10-second execution timeout.

```powershell
Invoke-WebRequest 'http://127.0.0.1:8443/api/snapshot' -Headers $headers -OutFile local-copy.png
Invoke-RestMethod 'http://127.0.0.1:8443/api/snapshot?out=window.png' -Headers $headers
dotnet run --project window-capture/apps/CaptureProbe/CaptureProbe.csproj -- --hwnd 657830 --out 'D:\captures\probe.png'
```

CLI snapshots retain local caller-selected paths. Streaming uses a single long-lived capture process and encoder, never one process per frame.

## Current distribution status

Branch pushes produce the `share-app-windows` Actions artifact containing `share-app-<run-number>-<commit-sha>.zip`. Tag pushes matching `v*` produce `share-app-<tag>.zip`, upload it as an Actions artifact for transfer, and attach it to the GitHub Release after the build and tests pass. Pull requests run build/test checks without publishing or uploading a distribution.

The ZIP contains `share-host.exe`, `CaptureProbe/`, `web/`, `.env.example`, README.md, LICENSE and `docs/`. It remains a **development distribution**: .NET 10 runtime and ffmpeg are external requirements. The host also resolves `ffmpeg.exe` beside its executable when supplied.

Self-contained publishing, a pinned ffmpeg bundle with checksum/notices, and a clean Windows 11 installation test are pending after Windows performance and device validation. See [remaining roadmap](docs/roadmap.md) for the remaining sequence.

WGC can stop updating a minimized window or a window on an inactive virtual desktop. Target switching cancels the old helper independently of frame arrival. Actual WGC capture, mixed-DPI click alignment, application-specific background input and iPhone behavior still require an interactive Windows/device test.

## Development and maintenance

Runtime changes must build in the order web UI → window capture → control server. Verify the Go executable path and version before building. Keep npm, NuGet and Go dependency locks in version control, and rebuild generated outputs locally. Do not commit dependency folders, build/publish output, captures, secrets, certificates or keys. Retain existing user features and the WGC/Win32 architecture; retained product strings are English.

Streaming ownership rules:

- Acquire the single control-connection slot before creating a peer; release it after processes, callbacks and held input have closed.
- Keep one long-lived CaptureProbe per target and one long-lived ffmpeg encoder. Snapshot helpers are separate bounded operations. Never start streaming processes per frame.
- Validate the 24-byte BGRA header, bound the latest-frame handoff, and keep reusable-buffer ownership explicit.
- Target switching must cancel capture without waiting for another frame. Consume RTCP, bound and synchronize diagnostics, and keep shutdown idempotent.
- Preserve the 10fps/quality baseline until interactive Windows measurements justify tuning. Serve only explicit client asset roots; never restore parent-directory fallback or loopback authorization bypasses.

Key implementation locations:

| Function | Source |
| --- | --- |
| Touch gestures and release | `web-ui/src/gestures.js` |
| Committed text, IME and special keys | `web-ui/src/keyboard.js` |
| Connection readiness, ICE and cleanup | `web-ui/src/webrtc.js` |
| Authentication refresh and retry | `web-ui/src/auth.js`, `web-ui/src/api.js` |
| Win32 coordinates, DPI, buttons and bounded calls | `control-server/internal/input/sendinput_windows.go`, `control-server/internal/input/target_windows.go` |
| Capture helper resolution and validated frame reads | `control-server/internal/nativecapture/bridge.go` |
| Capture/encoder ownership and pacing | `control-server/internal/webrtc/windowstream.go` |
| Native stream protocol | `window-capture/apps/CaptureProbe/Program.cs` |
| WGC lifecycle and copying | `window-capture/src/WindowCapture.Native/WgcCaptureService.cs` |

The control server resolves CaptureProbe from release `CaptureProbe/CaptureProbe.exe`, or development `window-capture/apps/CaptureProbe/bin/{Debug,Release}/net10.0-windows10.0.19041.0/win-x64/CaptureProbe.exe`. Do not claim Windows/iPhone compatibility, end-to-end performance gains or self-contained release readiness until the corresponding checks in [validation](docs/validation.md) and [roadmap](docs/roadmap.md) have passed.

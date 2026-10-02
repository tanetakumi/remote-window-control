# Share App

Control a single Windows application window from a phone or tablet. Share App streams the selected window to a mobile browser and sends touch and text input back to the Windows PC.

This repository is a substantially modified fork of [Share App - Remote Window Control](https://github.com/vpuhoff/remote-window-control) by **vpuhoff**. See [LICENSE](LICENSE) for attribution and license terms.

## Features

- Stream one selected application window instead of the entire desktop.
- Control it from a mobile browser with touch gestures.
- Choose between a relative pointer and direct tapping.
- Send text from the built-in text editor (up to 16 KiB per send) and use the special-key palette. The host pastes the text: it replaces its own clipboard with it and presses Ctrl+V in the target window, which is brought to the foreground first.
- Keep the session display alive with a loopback RDP connection, so capture continues after a remote viewer disconnects.
- Connect with WebRTC; no separate mobile app is required.

Only one control connection can be active at a time.

## Requirements

On the Windows PC:

- Windows 11 x64
- FFmpeg with the `libvpx-vp9` encoder, available on `PATH` or beside `share-host.exe`

The .NET runtime is bundled into `CaptureProbe.exe`; it does not need to be installed separately.

On the phone or tablet, use a browser that offers WebRTC VP9 profile 0. The host and browser must be able to reach each other over the network.

## Download and run

Download a Windows ZIP from [Releases](https://github.com/tanetakumi/remote-window-control/releases) when one is available. If no release has been published, build the Windows package from source using the instructions below.

1. Extract the ZIP into a folder. Keep `share-host.exe`, `CaptureProbe/`, `web/`, and `scripts/` together.
2. Install FFmpeg listed above.
3. Double-click `share-host.exe`. The packaged host runs in the background without a console window and displays a PC-and-phone icon in the Windows notification area; CaptureProbe and FFmpeg also run without console windows.
4. On the Windows PC, open <http://127.0.0.1:8443/> and select a window.

Closing the browser leaves the host running. Hover over the notification-area icon to see **Share App — 起動中**. Right-click it and choose **Open data folder（データフォルダーを開く）** to open `%LOCALAPPDATA%\ShareApp` in File Explorer, or **Exit（終了）** to stop the host and its active connections. If the icon is hidden, open the **^** overflow beside the clock. Diagnostics are saved to `%LOCALAPPDATA%\ShareApp\logs\share-host.log`.

The default address is available only from the Windows PC itself. To use the app from a phone, set up a trusted network path as described below.

## Phone access and security

Share App does not provide login or access control. By default, it listens on `127.0.0.1:8443`, so it is not directly reachable from another device. For phone access, use a private network or an authenticated HTTPS proxy such as Tailscale or Cloudflare Access, and restrict who can connect. Do not expose the host directly to the public internet.

For direct access from a trusted local network, see the [Windows setup guide](.github/distribution/はじめにお読みください.txt), which explains how to change the listen address and restrict the Windows Firewall rule.

## Build from source

The build script creates a Windows x64 distribution in `dist/share-app/` and runs the frontend and Go checks. Install these tools first:

- Node.js `>=22.12.0 <25`
- .NET SDK `10.0.401`
- Go `1.27.1`
- FFmpeg with the `libvpx-vp9` encoder

From the repository root, run:

```sh
node scripts/build.mjs
```

The output folder contains `share-host.exe`, `CaptureProbe/`, `web/`, `scripts/create-rdp-credentials.ps1`, and the license. The packaged host still requires FFmpeg on the Windows PC; the .NET runtime ships inside `CaptureProbe.exe`.

## Configuration

The host creates `%LOCALAPPDATA%\ShareApp\config.json` on first startup:

```json
{
  "listenAddr": "127.0.0.1:8443",
  "captureStats": "off",
  "fps": 8,
  "crf": 31,
  "maxScale": 2
}
```

Settings and logs are stored in this user directory for both installed and development builds. The installation directory only supplies executable programs and web assets. The host does not read `.env`, `SHARE_APP_*` configuration variables, or configuration files beside the executable, and does not migrate them.

| Setting | Default | Description |
| --- | --- | --- |
| `listenAddr` | `127.0.0.1:8443` | HTTP listen address. An empty host such as `:8443` listens on network interfaces too. HTTPS must be provided by a separate trusted proxy. |
| `captureStats` | `off` | Capture diagnostics: `off`, `on`, or `verify`. `verify` adds CPU-intensive pixel checks. Any value other than `off` also logs the browser's receive statistics. |

Stop the host before editing these fields and restart it to apply changes. The browser settings API only changes video settings; it preserves both startup fields. RDP credentials are stored separately in the optional encrypted file described below.

### Settings page

The gear button next to **Refresh** in the web UI opens a settings page with three options, shared by every device that connects:

| Setting | Default | Description |
| --- | --- | --- |
| Video FPS | `8` | Video encode rate in frames per second while the window changes, 1 to 30. Applies from the next connection. |
| Video quality (CRF) | `31` | VP9 constant rate factor, 0 to 63. Lower is sharper and uses more bandwidth; the `6M` bitrate cap stays fixed. Applies from the next connection. |
| Max window scale | `2` | Window pixels per CSS pixel of the browser viewport, 0.5 to 4 in steps of 0.25. The host sizes the shared window to the viewport times the device's pixel ratio, limited to this value, then fits it to the monitor's work area. Lower values give a smaller window (larger-looking UI) and less video data. Applies from the next connection or viewport change. |

**Save** writes the video settings to `%LOCALAPPDATA%\ShareApp\config.json`, preserving `listenAddr` and `captureStats`. Missing fields use their defaults. A missing file is created automatically; the host fails to start if the file is malformed or out of range.

A disconnected media connection gets a 5-second grace period to recover. Input commands are buffered while the send buffer is full; a backlog lasting 5 seconds or exceeding 256 queued commands ends the connection to avoid applying stale input.

## Session keep-alive (RDP)

When the Windows session is viewed over Remote Desktop and the human RDP client disconnects, the session is left without a display and Windows Graphics Capture stops. The optional session keep-alive holds a loopback RDP connection to the host's own session (`127.0.0.1:3389`, 1920x1080, received bitmaps discarded) so the session keeps a display and capture continues.

To use it:

1. Enable Remote Desktop on the Windows PC.
2. On that PC, as the Windows user who runs Share App, open Windows PowerShell in the extracted folder and run:

   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\create-rdp-credentials.ps1
   ```

   Enter the account's username and password (not a Windows Hello PIN). The password input is hidden. The script creates `%LOCALAPPDATA%\ShareApp\rdp-credentials.bin` and its parent directory if needed. The file contains UTF-8 JSON encrypted with Windows DPAPI in `CurrentUser` scope; it writes no plaintext credential file. The file is normally only decryptable by the same Windows user on that PC.

3. Restart Share App to load the credentials.
4. Press **Keep alive** in the web UI window list. The button shows a status dot that is polled every 5 seconds while the list is visible.

Credentials cannot be registered, read or changed through the web UI or HTTP API. Without the file, normal window sharing works and RDP keep-alive is unavailable. An existing file that cannot be decrypted or contains invalid data prevents startup and is reported in the log.

To replace credentials, stop the host, rerun the script with `-Force`, and restart. To disable keep-alive, stop the host and remove `%LOCALAPPDATA%\ShareApp\rdp-credentials.bin`. Generate the file on the PC and under the Windows user that will run the app.

While the keep-alive connection is up, the session display takes the requested 1920x1080 resolution. Any disconnect, including a human reconnecting and taking the session over, is terminal: the host never reconnects on its own, so press **Keep alive** again to start a new connection.

## Video encoding

Video uses VP9 profile 0 at 8 fps by default (the Video FPS setting), with low-latency screen encoding, CRF 31 (the Video quality setting) and target bitrate 6M. The target bitrate is not a limit on total network traffic or keyframe bursts. After a pixel change, the encoder runs at full rate for one second, then sends an idle delta frame about once per second. Browser PLI/FIR requests still trigger recovery keyframes.

The host logs the offer's codec lines to check browser VP9 support. A browser without VP9 profile 0 receives a connection error; there is no VP8 fallback. Before tuning quality or idle timing, compare static text, typing, menus and scrolling on Windows over a connection limited to about 1 Mbps, with `"captureStats": "on"` in `config.json` and the host restarted. Check host CPU, receive statistics and phone heat/battery use.

## Known limitation

Windows Graphics Capture may stop updating a minimized window or a window on an inactive virtual desktop. Restoring the window or moving it to the active desktop may resume capture.

## License and attribution

This fork retains attribution to **vpuhoff**, the author of the original project. The original work and this fork's modifications are offered under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). See [LICENSE](LICENSE) for details.

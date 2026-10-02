# Share App

Control a single Windows application window from a phone or tablet. Share App streams the selected window to a mobile browser and sends touch and text input back to the Windows PC.

This repository is a substantially modified fork of [Share App - Remote Window Control](https://github.com/vpuhoff/remote-window-control) by **vpuhoff**. See [LICENSE](LICENSE) for attribution and license terms.

## Features

- Stream one selected application window instead of the entire desktop.
- Control it from a mobile browser with touch gestures.
- Choose between a relative pointer and direct tapping.
- Send text from the built-in text editor and use the special-key palette.
- Connect with WebRTC; no separate mobile app is required.

Only one control connection can be active at a time.

## Requirements

On the Windows PC:

- Windows 11 x64
- [.NET 10 x64 Runtime](https://dotnet.microsoft.com/en-us/download/dotnet/10.0)
- FFmpeg with the `libvpx` encoder, available on `PATH` or beside `share-host.exe`

On the phone or tablet, use a modern browser. The host and browser must be able to reach each other over the network.

## Download and run

Download a Windows ZIP from [Releases](https://github.com/tanetakumi/remote-window-control/releases) when one is available. If no release has been published, build the Windows package from source using the instructions below.

1. Extract the ZIP into a folder. Keep `share-host.exe`, `CaptureProbe/`, `web/`, and `.env` together.
2. Install the .NET runtime and FFmpeg listed above.
3. Open PowerShell in the extracted folder and run:

   ```powershell
   .\share-host.exe
   ```

4. On the Windows PC, open <http://127.0.0.1:8443/> and select a window.

The default address is available only from the Windows PC itself. To use the app from a phone, set up a trusted network path as described below.

## Phone access and security

Share App does not provide login or access control. By default, it listens on `127.0.0.1:8443`, so it is not directly reachable from another device. For phone access, use a private network or an authenticated HTTPS proxy such as Tailscale or Cloudflare Access, and restrict who can connect. Do not expose the host directly to the public internet.

For direct access from a trusted local network, see the [Windows setup guide](.github/distribution/はじめにお読みください.txt), which explains how to change the listen address and restrict the Windows Firewall rule.

## Build from source

The build script creates a Windows x64 distribution in `dist/share-app/` and runs the frontend and Go checks. Install these tools first:

- Node.js `>=22.12.0 <25`
- .NET SDK `10.0.401`
- Go `1.27.1`
- FFmpeg with the `libvpx` encoder

From the repository root, run:

```sh
node scripts/build.mjs
```

The output folder contains `share-host.exe`, `CaptureProbe/`, `web/`, `.env`, and the license. The packaged host still requires the .NET 10 x64 Runtime and FFmpeg on the Windows PC.

## Configuration

The optional `.env` file sits beside `share-host.exe`. Environment variables override values in the file.

| Setting | Default | Description |
| --- | --- | --- |
| `SHARE_APP_ADDR` | `127.0.0.1:8443` | HTTP listen address. HTTPS must be provided by a separate trusted proxy. |
| `SHARE_APP_CAPTURE_STATS` | `off` | Capture diagnostics: `off`, `on`, or `verify`. `verify` adds CPU-intensive pixel checks. |

## Known limitation

Windows Graphics Capture may stop updating a minimized window or a window on an inactive virtual desktop. Restoring the window or moving it to the active desktop may resume capture.

## License and attribution

This fork retains attribution to **vpuhoff**, the author of the original project. The original work and this fork's modifications are offered under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). See [LICENSE](LICENSE) for details.

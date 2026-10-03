# Share App

Control a Windows application window from your phone or tablet. Share App streams one selected window over WebRTC and sends touch and text input back to the PC. No mobile app is required.

- Share a single window without streaming the entire desktop.
- Use relative-pointer or direct-tap controls, text input, and special keys.
- Adjust video quality, frame rate, and window scale from the browser.
- Optionally keep a Remote Desktop session active after disconnecting.

## Get started

You need **Windows 11 24H2 (build 26100 or later) x64** and a phone or tablet browser with **WebRTC VP9** support. FFmpeg and the .NET runtime are included.

1. Download `ShareApp-<version>-Setup.exe` from [Releases](https://github.com/tanetakumi/remote-window-control/releases) and run it. Installation is for the current Windows user and needs no administrator privileges.
2. Start **Share App** from the Start Menu. Its icon appears in the notification area beside the clock.
3. Open <http://127.0.0.1:8443/> on the PC to check that it is running.
4. Follow the [phone connection instructions](docs/usage.md#connect-from-a-phone), then open the app's address on your phone and select a window.

For a portable copy, download the ZIP, extract all its contents, and run `share-host.exe`.

Closing the browser leaves Share App running. Right-click its notification icon and choose **Exit** to stop it. Only one device can control a window at a time.

**Share App has no login.** Use a trusted private network and restrict access; do not expose it directly to the public internet. By default, only the PC itself can connect.

## Settings and help

The gear button beside **Refresh** opens the video settings. Settings and logs are stored in `%LOCALAPPDATA%\ShareApp`; use **Open data folder** from the notification icon to find them.

- [Connection, configuration, RDP keep-alive, and troubleshooting](docs/usage.md)
- [Build from source and create release packages](docs/build.md)
- [Report a problem](https://github.com/tanetakumi/remote-window-control/issues)

## Development

Install the [build prerequisites](docs/build.md#prerequisites) and prepare the bundled FFmpeg, then run from the repository root:

```sh
node scripts/build.mjs
```

This builds the app, runs its checks, and creates `dist/share-app/`. See the [build guide](docs/build.md) for installer packaging and GitHub Actions.

## License and attribution

This is a modified fork of [Share App - Remote Window Control](https://github.com/vpuhoff/remote-window-control) by **vpuhoff**, maintained by **tanetakumi**. The original work and this fork's modifications are licensed under [CC BY 4.0](LICENSE).

The bundled FFmpeg is **LGPL-2.1-or-later** and libvpx is **BSD**. Packages include their notices; matching sources and build instructions are provided as `ffmpeg-<version>-share-app-sources.tar.gz` alongside the binaries. See the [FFmpeg build guide](scripts/ffmpeg/README.md).

# Build and release

## Prerequisites

- Node.js `>=22.12.0 <25`
- .NET SDK `10.0.401` (pinned in `global.json`)
- Go `1.27.1` (pinned in `control-server/go.mod`)
- The bundled Windows FFmpeg, prepared using the [FFmpeg build guide](../scripts/ffmpeg/README.md)

Build FFmpeg on Ubuntu/WSL, or download the `share-app-ffmpeg` GitHub Actions artifact and extract it into `dist/ffmpeg/`. Keep its package, source archive, and checksum file together.

Linux app builds also need a native FFmpeg with `libvpx-vp9` on PATH for integration tests. Windows builds use the bundled executable for those tests.

## Build the app

From the repository root:

```sh
node scripts/build.mjs
```

The script builds the web UI, CaptureProbe, and Windows host, runs frontend and Go checks, and assembles `dist/share-app/`. `--check` runs the build and checks without assembling the distribution. Linux builds skip the Windows-only window enumeration check.

## Create packages

On Windows:

```powershell
./installer/build.ps1 -Version 0.1.2
```

The script downloads a SHA-256-pinned Inno Setup compiler on first use. To use an existing compiler, pass `-Compiler <path-to-ISCC.exe>`.

`dist/releases/` contains the Setup EXE, portable ZIP, corresponding FFmpeg/libvpx sources, `SHA256SUMS.txt`, and release notes. Generated binaries are not committed to Git.

The installer definition is `installer/share-app.iss`. To check installation, upgrade, uninstallation, and user data preservation, run on a disposable Windows profile:

```powershell
./installer/test.ps1 -InstallerPath ./dist/releases/ShareApp-0.1.2-Setup.exe
```

## GitHub Actions and releases

The **Build** workflow uses the same scripts:

- Pull requests build and test the app.
- Manual runs also package and test the installer. Download the `share-app-windows-packages` artifact for the results.
- Pushing a new `v<major>.<minor>.<patch>` tag also creates a draft GitHub Release with the packages. Prerelease suffixes are supported.

Confirm launch and actual screen sharing on Windows 11 24H2 or later before publishing the draft. Actions artifacts expire after 14 days; published Releases provide lasting downloads. Keep the corresponding FFmpeg source archive available alongside its binaries.

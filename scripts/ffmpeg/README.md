# Bundled FFmpeg

Share App builds a separate Windows x64 FFmpeg executable from the unmodified
archives pinned in `scripts/ffmpeg/versions.env`. Only VP9 encoding/decoding,
raw video, pixel conversion, IVF, and pipes are enabled. Decoding supports the
integration tests that check the encoded keyframes. GPL, nonfree,
network access, and external-library autodetection are disabled. libvpx is
the only external media library. FFmpeg remains LGPL-2.1-or-later; libvpx is BSD.

## Build / rebuild

On Ubuntu 24.04 (also works on later Ubuntu), install the cross toolchain:

```sh
sudo apt-get update
sudo apt-get install --no-install-recommends gcc-mingw-w64-x86-64 \
  mingw-w64-tools nasm make pkg-config curl xz-utils
bash scripts/ffmpeg/build.sh
```

The output is `dist/ffmpeg/package/` and a separate corresponding-source archive.
Copy the **whole package**, including notices, into the application distribution.
Do not substitute an FFmpeg from PATH into a distributable build.

To rebuild from a downloaded source bundle, extract it, open its root directory,
and use the included archives without downloading media sources again:

```sh
FFMPEG_SOURCE_DIR="$PWD/downloads" bash scripts/ffmpeg/build.sh output
```

`BUILD_JOBS` controls make parallelism. `BUILD-INFO.txt`, `config.h`, and
`config.mak` record the original build environment and configuration; source
archives contain all upstream licenses. The compiler packages include MinGW
and GCC runtime components with their respective license exceptions. Their
copyright texts are also included. No upstream patches are applied.

## Updating

Review both upstream releases, update the version / libvpx commit and SHA-256
pins together, and rebuild. The source build checks for third-party DLL imports.
Windows app builds verify the executable's LGPL license, configuration, and VP9 encoder.
The application's integration tests run against this same executable.

The source archive must travel with every installer/ZIP artifact and Release.
GitHub's automatic repository source ZIP is **not** the FFmpeg source bundle.
Never remove the corresponding sources while the binaries remain available.

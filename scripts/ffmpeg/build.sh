#!/usr/bin/env bash
# Build a standalone Windows x64 LGPL FFmpeg executable and its source bundle.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$script_dir/versions.env"
output=$(realpath -m -- "${1:-$script_dir/../../dist/ffmpeg}")
downloads=$(realpath -m -- "${FFMPEG_SOURCE_DIR:-$output/downloads}")
mkdir -p "$output" "$downloads"
work=$(mktemp -d "$output/.build-XXXXXX")
trap 'rm -rf -- "$work"' EXIT
jobs=${BUILD_JOBS:-$(nproc)}

fetch() {
  local url=$1 name=$2 expected=$3
  if [[ ! -f "$downloads/$name" ]]; then
    curl --fail --location --retry 3 "$url" -o "$downloads/$name.part"
    mv -- "$downloads/$name.part" "$downloads/$name"
  fi
  printf '%s  %s\n' "$expected" "$downloads/$name" | sha256sum --check --status
}
fetch "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz" \
  "ffmpeg-$FFMPEG_VERSION.tar.xz" "$FFMPEG_SHA256"
fetch "https://codeload.github.com/webmproject/libvpx/tar.gz/$LIBVPX_REVISION" \
  "libvpx-$LIBVPX_VERSION.tar.gz" "$LIBVPX_SHA256"
tar -xf "$downloads/ffmpeg-$FFMPEG_VERSION.tar.xz" -C "$work"
tar -xf "$downloads/libvpx-$LIBVPX_VERSION.tar.gz" -C "$work"
prefix="$work/prefix"
export CROSS=x86_64-w64-mingw32-

cd "$work/libvpx-$LIBVPX_REVISION"
./configure --prefix="$prefix" --target=x86_64-win64-gcc --as=nasm \
  --disable-examples --disable-tools --disable-docs --disable-unit-tests \
  --disable-vp8 --disable-vp9-decoder --disable-shared --enable-static
make -j "$jobs"
make install

cd "$work/ffmpeg-$FFMPEG_VERSION"
export PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig"
./configure --prefix="$prefix" --arch=x86_64 --target-os=mingw32 \
  --cross-prefix="$CROSS" --enable-cross-compile \
  --pkg-config=pkg-config --pkg-config-flags=--static \
  --disable-autodetect --disable-everything --disable-network \
  --disable-doc --disable-debug --disable-ffplay --disable-ffprobe \
  --disable-shared --enable-static --disable-gpl --disable-nonfree \
  --enable-libvpx --enable-encoder=libvpx_vp9,rawvideo --enable-decoder=rawvideo,vp9 \
  --enable-demuxer=rawvideo,ivf --enable-muxer=ivf,rawvideo --enable-protocol=pipe \
  --enable-filter=format,scale --extra-ldflags=-static
grep -q '^#define FFMPEG_LICENSE "LGPL version 2.1 or later"' config.h
make -j "$jobs" ffmpeg.exe
# The package must not rely on DLLs from the build machine or a global FFmpeg.
imports=$(x86_64-w64-mingw32-objdump -p ffmpeg.exe | sed -n 's/.*DLL Name: //p' | tr '[:upper:]' '[:lower:]')
if printf '%s\n' "$imports" | grep -Ev '^(kernel32|msvcrt|user32|advapi32|bcrypt|ole32|shell32)\.dll$'; then
  printf 'Unexpected non-system FFmpeg DLL import\n' >&2
  exit 1
fi

# Recreate the distributable output; never reuse a binary or source bundle from
# a previous build. Keep the verified download cache separately.
rm -rf -- "$output/package" "$output/source"
mkdir -p "$output/package/licenses/ffmpeg" "$output/source/scripts/ffmpeg" "$output/source/downloads"
cp ffmpeg.exe "$output/package/ffmpeg.exe"
cp COPYING.LGPLv2.1 LICENSE.md "$output/package/licenses/ffmpeg/"
cp "$work/libvpx-$LIBVPX_REVISION/"{LICENSE,PATENTS,AUTHORS} "$output/package/licenses/ffmpeg/"
cp "$script_dir/NOTICE.txt" "$output/package/licenses/ffmpeg/NOTICE.txt"
cp "$script_dir/build.sh" "$output/source/scripts/ffmpeg/"
cp "$script_dir/"{versions.env,README.md,NOTICE.txt} "$output/source/scripts/ffmpeg/"
cp "$downloads/ffmpeg-$FFMPEG_VERSION.tar.xz" "$downloads/libvpx-$LIBVPX_VERSION.tar.gz" "$output/source/downloads/"
cp config.h ffbuild/config.mak "$output/source/"
{
  printf 'Unmodified FFmpeg %s; unmodified libvpx %s (%s).\n\n' "$FFMPEG_VERSION" "$LIBVPX_VERSION" "$LIBVPX_REVISION"
  x86_64-w64-mingw32-gcc --version
  nasm --version
  make --version
  pkg-config --version
  printf '\nInstalled build packages:\n'
  dpkg-query -W 'gcc-mingw-w64-x86-64*' 'mingw-w64*' nasm make pkg-config 2>/dev/null || true
} > "$output/source/BUILD-INFO.txt"
# Document the toolchain runtime license exceptions alongside the media notices.
for document in /usr/share/doc/gcc-mingw-w64-x86-64*/copyright /usr/share/doc/mingw-w64-common/copyright; do
  if [[ -f "$document" ]]; then
    name=$(basename -- "$(dirname -- "$document")")
    cp "$document" "$output/package/licenses/ffmpeg/$name-copyright.txt"
  fi
done
cp -r "$output/package/licenses" "$output/source/"
cp "$script_dir/README.md" "$output/source/README.md"
tar -czf "$output/ffmpeg-$FFMPEG_VERSION-share-app-sources.tar.gz" -C "$output/source" .
(cd "$output" && sha256sum "ffmpeg-$FFMPEG_VERSION-share-app-sources.tar.gz" > sources.sha256)
(cd "$output/package" && sha256sum ffmpeg.exe > ffmpeg.sha256)
printf '\nFFmpeg and corresponding sources ready: %s\n' "$output"

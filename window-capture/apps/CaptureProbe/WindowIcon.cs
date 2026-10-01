using System.Runtime.InteropServices;
using SixLabors.ImageSharp;
using SixLabors.ImageSharp.PixelFormats;

namespace CaptureProbe;

internal static class WindowIcon
{
    private const int Size = 32;
    private const uint MessageTimeoutMs = 50;
    private const uint WM_GETICON = 0x007F;
    private const nuint ICON_BIG = 1;
    private const nuint ICON_SMALL2 = 2;
    private const uint SMTO_ABORTIFHUNG = 0x0002;
    private const uint SMTO_ERRORONEXIT = 0x0020;
    private const int GCLP_HICON = -14;
    private const int GCLP_HICONSM = -34;
    private const uint DI_NORMAL = 3;

    public static string? GetPng(nint hwnd)
    {
        // Never wait indefinitely for another application's window procedure.
        nint source = 0;
        foreach (nuint kind in new nuint[] { ICON_BIG, ICON_SMALL2 })
        {
            if (SendMessageTimeoutW(hwnd, WM_GETICON, kind, 0, SMTO_ABORTIFHUNG | SMTO_ERRORONEXIT, MessageTimeoutMs, out var result) == 0) break;
            source = (nint)result;
            if (source != 0) break;
        }
        if (source == 0) source = GetClassLongPtrW(hwnd, GCLP_HICON);
        if (source == 0) source = GetClassLongPtrW(hwnd, GCLP_HICONSM);
        if (source == 0) return null;

        // The window owns the original HICON; only destroy our private copy.
        var icon = CopyIcon(source);
        if (icon == 0) return null;
        nint dc = 0, bitmap = 0, previous = 0;
        try
        {
            dc = CreateCompatibleDC(0);
            if (dc == 0) return null;
            var info = new BitmapInfo
            {
                HeaderSize = 40, Width = Size, Height = -Size, Planes = 1, BitCount = 32,
            };
            bitmap = CreateDIBSection(dc, ref info, 0, out var bits, 0, 0);
            if (bitmap == 0) return null;
            previous = SelectObject(dc, bitmap);
            if (previous == 0 || previous == -1) return null;

            var black = new byte[Size * Size * 4];
            var white = new byte[black.Length];
            Marshal.Copy(black, 0, bits, black.Length);
            if (!DrawIconEx(dc, 0, 0, icon, Size, Size, 0, 0, DI_NORMAL)) return null;
            GdiFlush();
            Marshal.Copy(bits, black, 0, black.Length);
            Array.Fill(white, (byte)255);
            Marshal.Copy(white, 0, bits, white.Length);
            if (!DrawIconEx(dc, 0, 0, icon, Size, Size, 0, 0, DI_NORMAL)) return null;
            GdiFlush();
            Marshal.Copy(bits, white, 0, white.Length);

            // Black/white compositing recovers transparency for both alpha and legacy mask icons.
            for (var i = 0; i < black.Length; i += 4)
            {
                var alpha = Math.Clamp(255 - white[i] + black[i], 0, 255);
                for (var channel = 0; channel < 3; channel++)
                    black[i + channel] = alpha == 0 ? (byte)0 : (byte)Math.Min(255, black[i + channel] * 255 / alpha);
                black[i + 3] = (byte)alpha;
            }
            using var image = Image.LoadPixelData<Bgra32>(black, Size, Size);
            using var output = new MemoryStream();
            image.SaveAsPng(output);
            return Convert.ToBase64String(output.GetBuffer(), 0, (int)output.Length);
        }
        finally
        {
            if (previous != 0 && previous != -1) SelectObject(dc, previous);
            if (bitmap != 0) DeleteObject(bitmap);
            if (dc != 0) DeleteDC(dc);
            DestroyIcon(icon);
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct BitmapInfo
    {
        public uint HeaderSize;
        public int Width, Height;
        public ushort Planes, BitCount;
        public uint Compression, ImageSize;
        public int XPelsPerMeter, YPelsPerMeter;
        public uint ColorsUsed, ColorsImportant, ColorTable;
    }

    [DllImport("user32.dll")]
    private static extern nint SendMessageTimeoutW(nint hwnd, uint message, nuint wParam, nint lParam, uint flags, uint timeout, out nuint result);
    [DllImport("user32.dll")]
    private static extern nint GetClassLongPtrW(nint hwnd, int index);
    [DllImport("user32.dll")]
    private static extern nint CopyIcon(nint icon);
    [DllImport("user32.dll")]
    private static extern bool DestroyIcon(nint icon);
    [DllImport("user32.dll")]
    private static extern bool DrawIconEx(nint dc, int x, int y, nint icon, int width, int height, uint step, nint brush, uint flags);
    [DllImport("gdi32.dll")]
    private static extern nint CreateCompatibleDC(nint dc);
    [DllImport("gdi32.dll")]
    private static extern nint CreateDIBSection(nint dc, ref BitmapInfo info, uint usage, out nint bits, nint section, uint offset);
    [DllImport("gdi32.dll")]
    private static extern nint SelectObject(nint dc, nint value);
    [DllImport("gdi32.dll")]
    private static extern bool DeleteObject(nint value);
    [DllImport("gdi32.dll")]
    private static extern bool DeleteDC(nint dc);
    [DllImport("gdi32.dll")]
    private static extern bool GdiFlush();
}

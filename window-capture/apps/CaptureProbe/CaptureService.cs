using SixLabors.ImageSharp;
using SixLabors.ImageSharp.PixelFormats;

namespace CaptureProbe;

internal static class CaptureService
{
    private static readonly bool DebugLoggingEnabled =
        string.Equals(Environment.GetEnvironmentVariable("WINDOW_CAPTURE_DEBUG"), "1", StringComparison.Ordinal);

    public static void CaptureWindowToPng(nint hwnd, Stream output)
    {
        using var image = CaptureWindowImage(hwnd);
        LogDebug("Encoding PNG to output");
        image.SaveAsPng(output);
        LogDebug("Encoded PNG to output");
    }

    private static Image<Bgra32> CaptureWindowImage(nint hwnd)
    {
        LogDebug($"Capture start hwnd={hwnd}");
        if (!WgcCaptureService.IsSupported())
        {
            throw new PlatformNotSupportedException("Windows.Graphics.Capture is not supported on this system.");
        }

        using var capture = new WgcCaptureService();
        capture.StartCapture(hwnd);

        var frameId = capture.WaitForFrame(0, 3000);
        if (frameId <= 0)
        {
            throw new TimeoutException("Timed out waiting for the first capture frame.");
        }

        var buffer = Array.Empty<byte>();
        var frame = capture.CopyLatestFrame(ref buffer)
            ?? throw new InvalidOperationException("Capture frame is unavailable.");

        return Image.LoadPixelData<Bgra32>(
            buffer.AsSpan(0, frame.BytesWritten),
            frame.Width,
            frame.Height);
    }

    private static void LogDebug(string message)
    {
        if (DebugLoggingEnabled)
        {
            Console.Error.WriteLine($"[capture] {message}");
        }
    }
}

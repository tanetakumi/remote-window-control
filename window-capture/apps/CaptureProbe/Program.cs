using System.Buffers.Binary;
using System.Text.Json;
using CaptureProbe;

string? mode = null;
nint? hwnd = null;
string? stats = null;
bool includeSecondaryWindows = false;
for (var i = 0; i < args.Length; i++)
{
    switch (args[i].ToLowerInvariant())
    {
        case "--list":
        case "--stream":
        case "--stdout-png":
            if (mode is not null) return Usage();
            mode = args[i].ToLowerInvariant();
            break;
        case "--hwnd":
            if (hwnd is not null || ++i >= args.Length ||
                !long.TryParse(args[i], out var handle) || handle == 0)
            {
                return Usage();
            }
            hwnd = (nint)handle;
            break;
        case "--stats":
        case "--stats-verify":
            if (stats is not null) return Usage();
            stats = args[i].ToLowerInvariant();
            break;
        case "--include-secondary-windows":
            if (includeSecondaryWindows) return Usage();
            includeSecondaryWindows = true;
            break;
        default:
            return Usage();
    }
}

if (mode is null || (mode == "--list" ? hwnd is not null : hwnd is null) ||
    (mode != "--stream" && (stats is not null || includeSecondaryWindows)))
{
    return Usage();
}

if (mode == "--list")
{
    Console.WriteLine(JsonSerializer.Serialize(
        WindowEnumeration.ListVisibleWindows().Select(window => new
        {
            handle = window.Handle,
            title = window.Title,
            process_id = window.ProcessId,
            process_name = window.ProcessName,
            class_name = window.ClassName,
            icon_png = window.IconPng,
        }),
        new JsonSerializerOptions
        {
            WriteIndented = true,
        }));
    return 0;
}

using var stdout = Console.OpenStandardOutput();
if (mode == "--stream")
{
    var captureStats = stats is null ? null : new CaptureStats(stats == "--stats-verify", Console.Error);
    StreamFrames(hwnd!.Value, stdout, captureStats, includeSecondaryWindows);
}
else
{
    CaptureService.CaptureWindowToPng(hwnd!.Value, stdout);
}
return 0;

static int Usage()
{
    Console.Error.WriteLine("usage: CaptureProbe --list | --hwnd <handle> --stream [--stats | --stats-verify] [--include-secondary-windows] | --hwnd <handle> --stdout-png");
    return 1;
}

// stats, when given, writes dirty-region measurements to stderr; stdout carries
// only frames.
static void StreamFrames(nint hwnd, Stream stdout, CaptureStats? stats, bool includeSecondaryWindows)
{
    using var capture = new WgcCaptureService(stats);
    capture.StartCapture(hwnd, includeSecondaryWindows);

    var header = new byte[24];
    var frameBuffer = Array.Empty<byte>();
    long lastSeenFrameId = 0;

    while (true)
    {
        stats?.ReportIfDue();
        if (capture.WaitForFrame(lastSeenFrameId, 1000) <= 0)
        {
            continue;
        }

        if (capture.CopyLatestFrame(ref frameBuffer) is not { } frame)
        {
            continue;
        }

        BinaryPrimitives.WriteUInt32LittleEndian(header.AsSpan(0, 4), (uint)frame.BytesWritten);
        BinaryPrimitives.WriteInt32LittleEndian(header.AsSpan(4, 4), frame.Width);
        BinaryPrimitives.WriteInt32LittleEndian(header.AsSpan(8, 4), frame.Height);
        BinaryPrimitives.WriteInt32LittleEndian(header.AsSpan(12, 4), frame.Stride);
        BinaryPrimitives.WriteInt64LittleEndian(header.AsSpan(16, 8), frame.FrameId);

        lastSeenFrameId = frame.FrameId;
        stats?.RecordStreamed();
        stdout.Write(header);
        stdout.Write(frameBuffer.AsSpan(0, frame.BytesWritten));
        stdout.Flush();
    }
}

using System.Diagnostics;
using System.Globalization;
using System.Runtime.InteropServices;
using Windows.Graphics;

namespace CaptureProbe;

// Aggregates per-WGC-frame dirty-region statistics and writes one summary line
// to stderr per interval, so measurements never mix with the pixel stream on
// stdout and stay small enough for the host log. A line is written even when
// no frame arrived, which tells a static window apart from a stopped capture.
//
// Frames whose dirty regions cannot be trusted (the first frame, the first one
// after a resize, or when the API is unavailable) are counted as unknown,
// never as unchanged.
//
// With verification enabled each frame is also compared with the previous one
// pixel by pixel, to check that changed pixels lie inside the reported regions.
internal sealed class CaptureStats
{
    private static readonly long ReportInterval = Stopwatch.Frequency * 5;

    private readonly object _lock = new();
    private readonly bool _verify;
    private readonly TextWriter _output;

    private string _dirtyMode = "off";
    private long _intervalStart = Stopwatch.GetTimestamp();
    private long _lastFrameAt;
    private int _width;
    private int _height;
    private bool _resync = true;
    private Interval _current;

    private byte[] _previous = [];
    private bool _previousValid;
    private byte[] _mask = [];

    public CaptureStats(bool verify, TextWriter output)
    {
        _verify = verify;
        _output = output;
    }

    public void Start(string dirtyMode)
    {
        lock (_lock)
        {
            _dirtyMode = dirtyMode;
        }
    }

    // The capture is being recreated for a new size; the next frame starts a
    // new generation whose regions do not describe a change from this one.
    public void MarkResized()
    {
        lock (_lock)
        {
            _current.Resized++;
            _resync = true;
            _previousValid = false;
        }
    }

    public void RecordStreamed()
    {
        lock (_lock)
        {
            _current.Streamed++;
        }
    }

    // pixels is the stored BGRA frame (width*4 bytes per row). regions is null
    // when dirty regions are unavailable for this frame.
    public void RecordFrame(int width, int height, IReadOnlyList<RectInt32>? regions, long copyTicks, ReadOnlySpan<byte> pixels)
    {
        lock (_lock)
        {
            var now = Stopwatch.GetTimestamp();
            ref var s = ref _current;
            s.Frames++;
            if (_lastFrameAt != 0)
            {
                s.GapMax = Math.Max(s.GapMax, now - _lastFrameAt);
            }
            _lastFrameAt = now;
            s.CopyTotal += copyTicks;
            s.CopyMax = Math.Max(s.CopyMax, copyTicks);

            if (width != _width || height != _height)
            {
                _width = width;
                _height = height;
                _resync = true;
                _previousValid = false;
            }

            var known = regions is not null && !_resync;
            _resync = false;
            var frameArea = (long)width * height;
            Span<Rect> clipped = [];
            if (known)
            {
                clipped = Clip(regions!, width, height);
                var area = UnionArea(clipped);
                s.Known++;
                s.RectsTotal += regions!.Count;
                s.RectsMax = Math.Max(s.RectsMax, regions.Count);
                var ratio = (double)area / frameArea;
                s.AreaRatioTotal += ratio;
                s.AreaRatioMax = Math.Max(s.AreaRatioMax, ratio);
                if (area == 0) s.Empty++;
                else if (area >= frameArea) s.Full++;
                else if (ratio <= 0.01) s.Le1++;
                else if (ratio <= 0.10) s.Le10++;
                else if (ratio <= 0.50) s.Le50++;
                else s.Lt100++;
            }
            else
            {
                s.Unknown++;
            }

            if (_verify)
            {
                Verify(width, height, pixels, known, clipped);
            }
        }
    }

    // Writes a summary when the interval has elapsed. Called from the stream
    // loop, which wakes at least once a second.
    public void ReportIfDue()
    {
        string line;
        lock (_lock)
        {
            var now = Stopwatch.GetTimestamp();
            if (now - _intervalStart < ReportInterval)
            {
                return;
            }
            line = Format(now - _intervalStart);
            _intervalStart = now;
            _current = default;
        }
        _output.WriteLine(line);
    }

    private void Verify(int width, int height, ReadOnlySpan<byte> pixels, bool known, ReadOnlySpan<Rect> regions)
    {
        var length = checked(width * height * 4);
        ref var s = ref _current;
        if (_previousValid && _previous.Length >= length)
        {
            if (known)
            {
                if (_mask.Length < width * height) _mask = new byte[width * height];
                var mask = _mask.AsSpan(0, width * height);
                mask.Clear();
                foreach (var r in regions)
                {
                    for (var y = r.Top; y < r.Bottom; y++)
                    {
                        mask.Slice(y * width + r.Left, r.Right - r.Left).Fill(1);
                    }
                }
            }

            long changed = 0, missed = 0;
            var stride = width * 4;
            for (var y = 0; y < height; y++)
            {
                var current = pixels.Slice(y * stride, stride);
                var previous = _previous.AsSpan(y * stride, stride);
                if (current.SequenceEqual(previous))
                {
                    continue;
                }
                var currentPixels = MemoryMarshal.Cast<byte, uint>(current);
                var previousPixels = MemoryMarshal.Cast<byte, uint>(previous);
                for (var x = 0; x < width; x++)
                {
                    if (currentPixels[x] == previousPixels[x]) continue;
                    changed++;
                    if (known && _mask[y * width + x] == 0) missed++;
                }
            }

            s.Verified++;
            s.ChangedRatioTotal += (double)changed / ((long)width * height);
            if (changed == 0) s.Unchanged++;
            if (missed > 0)
            {
                s.MissedFrames++;
                s.MissedPixels += missed;
            }
        }

        if (_previous.Length < length) _previous = new byte[length];
        pixels[..length].CopyTo(_previous);
        _previousValid = true;
    }

    private string Format(long elapsed)
    {
        var s = _current;
        var ic = CultureInfo.InvariantCulture;
        var line = string.Format(ic,
            "[capture-stats] interval_ms={0:F0} dirty={1} size={2}x{3} frames={4} streamed={5} resized={6} " +
            "unknown={7} empty={8} le1={9} le10={10} le50={11} lt100={12} full={13} " +
            "rects_avg={14:F1} rects_max={15} area_pct_avg={16:F2} area_pct_max={17:F2} " +
            "gap_ms_max={18:F0} copy_ms_avg={19:F2} copy_ms_max={20:F2}",
            Ms(elapsed), _dirtyMode, _width, _height, s.Frames, s.Streamed, s.Resized,
            s.Unknown, s.Empty, s.Le1, s.Le10, s.Le50, s.Lt100, s.Full,
            s.Known > 0 ? (double)s.RectsTotal / s.Known : 0, s.RectsMax,
            s.Known > 0 ? s.AreaRatioTotal * 100 / s.Known : 0, s.AreaRatioMax * 100,
            Ms(s.GapMax), s.Frames > 0 ? Ms(s.CopyTotal) / s.Frames : 0, Ms(s.CopyMax));
        if (_verify)
        {
            line += string.Format(ic,
                " verified={0} unchanged={1} changed_pct_avg={2:F2} missed_frames={3} missed_px={4}",
                s.Verified, s.Unchanged, s.Verified > 0 ? s.ChangedRatioTotal * 100 / s.Verified : 0,
                s.MissedFrames, s.MissedPixels);
        }
        return line;
    }

    private static double Ms(long ticks) => ticks * 1000.0 / Stopwatch.Frequency;

    private static Span<Rect> Clip(IReadOnlyList<RectInt32> regions, int width, int height)
    {
        var result = new Rect[regions.Count];
        var count = 0;
        foreach (var r in regions)
        {
            var left = Math.Clamp(r.X, 0, width);
            var top = Math.Clamp(r.Y, 0, height);
            var right = Math.Clamp((long)r.X + r.Width, 0, width);
            var bottom = Math.Clamp((long)r.Y + r.Height, 0, height);
            if (right > left && bottom > top)
            {
                result[count++] = new Rect(left, top, (int)right, (int)bottom);
            }
        }
        return result.AsSpan(0, count);
    }

    // Area covered by the rectangles, counting overlaps once: sweep the
    // vertical strips between distinct x edges and merge y intervals per strip.
    internal static long UnionArea(ReadOnlySpan<Rect> rects)
    {
        if (rects.Length == 0) return 0;
        var xs = new List<int>(rects.Length * 2);
        foreach (var r in rects)
        {
            xs.Add(r.Left);
            xs.Add(r.Right);
        }
        xs.Sort();
        var intervals = new List<(int Top, int Bottom)>(rects.Length);
        long area = 0;
        for (var i = 0; i + 1 < xs.Count; i++)
        {
            var left = xs[i];
            var right = xs[i + 1];
            if (right == left) continue;
            intervals.Clear();
            foreach (var r in rects)
            {
                if (r.Left <= left && r.Right >= right) intervals.Add((r.Top, r.Bottom));
            }
            if (intervals.Count == 0) continue;
            intervals.Sort();
            long covered = 0;
            var (top, bottom) = intervals[0];
            foreach (var (t, b) in intervals)
            {
                if (t > bottom)
                {
                    covered += bottom - top;
                    (top, bottom) = (t, b);
                }
                else if (b > bottom)
                {
                    bottom = b;
                }
            }
            covered += bottom - top;
            area += covered * (right - left);
        }
        return area;
    }

    internal readonly record struct Rect(int Left, int Top, int Right, int Bottom);

    private struct Interval
    {
        public int Frames, Streamed, Resized, Unknown, Known;
        public int Empty, Le1, Le10, Le50, Lt100, Full;
        public long RectsTotal;
        public int RectsMax;
        public double AreaRatioTotal, AreaRatioMax;
        public long GapMax, CopyTotal, CopyMax;
        public int Verified, Unchanged, MissedFrames;
        public double ChangedRatioTotal;
        public long MissedPixels;
    }
}

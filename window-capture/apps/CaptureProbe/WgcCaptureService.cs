using System.Buffers;
using System.Diagnostics;
using System.Runtime.InteropServices;
using Vortice.Direct3D;
using Vortice.Direct3D11;
using WinRT;
using Windows.Foundation.Metadata;
using Windows.Graphics;
using Windows.Graphics.Capture;
using Windows.Graphics.DirectX;
using Windows.Graphics.DirectX.Direct3D11;

namespace CaptureProbe;

internal sealed class WgcCaptureService : IDisposable
{
    private ID3D11Device? _d3dDevice;
    private IDirect3DDevice? _winrtDevice;
    private GraphicsCaptureItem? _captureItem;
    private Direct3D11CaptureFramePool? _framePool;
    private GraphicsCaptureSession? _session;
    private ID3D11Texture2D? _stagingTexture;
    private readonly object _frameLock = new();
    private readonly object _captureLock = new();
    private readonly AutoResetEvent _frameEvent = new(false);
    private byte[]? _frameBuffer;
    private int _captureWidth;
    private int _captureHeight;
    private FrameInfo _frame;
    private Exception? _fatalError;
    private volatile bool _disposed;
    private readonly CaptureStats? _stats;
    private bool _reportDirtyRegions;
    private bool _dirtyRegionErrorLogged;

    // stats, when given, receives every WGC frame.
    public WgcCaptureService(CaptureStats? stats = null)
    {
        _stats = stats;
    }

    // Dirty regions arrived in Windows 11 24H2 (SDK 26100). Probe the API
    // itself rather than the OS build.
    private static readonly Lazy<bool> DirtyRegionsPresent = new(() =>
        ApiInformation.IsPropertyPresent("Windows.Graphics.Capture.GraphicsCaptureSession", "DirtyRegionMode") &&
        ApiInformation.IsPropertyPresent("Windows.Graphics.Capture.Direct3D11CaptureFrame", "DirtyRegions"));

    public static bool IsSupported()
    {
        GraphicsCaptureInterop.EnsureInitialized();
        return GraphicsCaptureSession.IsSupported();
    }

    public void StartCapture(IntPtr hwnd, bool includeSecondaryWindows = false)
    {
        ThrowIfDisposed();
        GraphicsCaptureInterop.EnsureInitialized();
        InitializeDevices();
        _captureItem = GraphicsCaptureInterop.CreateItemForWindow(hwnd);
        _captureItem.Closed += OnCaptureClosed;
        _captureWidth = _captureItem.Size.Width;
        _captureHeight = _captureItem.Size.Height;
        _framePool = Direct3D11CaptureFramePool.CreateFreeThreaded(
            _winrtDevice!,
            DirectXPixelFormat.B8G8R8A8UIntNormalized,
            2,
            _captureItem.Size);

        _framePool.FrameArrived += OnFrameArrived;
        _session = _framePool.CreateCaptureSession(_captureItem);
        // The browser draws its own cursor overlay; the host cursor would only
        // add frames and changed pixels to the stream.
        _session.IsCursorCaptureEnabled = false;
        // PC mode: owned popups such as menus, clipped to the window's bounds.
        _session.IncludeSecondaryWindows = includeSecondaryWindows;
        _stats?.Start(EnableDirtyRegions());
        _session.StartCapture();
    }

    public long WaitForFrame(long lastSeenFrameId, int timeoutMs)
    {
        ThrowIfDisposed();
        ThrowIfFaulted();

        lock (_frameLock)
        {
            if (_frame.FrameId > lastSeenFrameId)
            {
                return _frame.FrameId;
            }
        }

        var signaled = _frameEvent.WaitOne(timeoutMs);
        ThrowIfFaulted();
        if (!signaled)
        {
            return 0;
        }

        lock (_frameLock)
        {
            return _frame.FrameId > lastSeenFrameId ? _frame.FrameId : 0;
        }
    }

    // Grow and copy under the same lock so resizing cannot invalidate a size probe.
    // The caller owns the returned pixels and can reuse its buffer on the next call.
    public FrameInfo? CopyLatestFrame(ref byte[] buffer)
    {
        ThrowIfDisposed();
        ThrowIfFaulted();

        lock (_frameLock)
        {
            ThrowIfDisposed();
            if (_frame.FrameId == 0 || _frameBuffer is null)
            {
                return null;
            }

            if (buffer.Length < _frame.BytesWritten)
            {
                buffer = new byte[_frame.BytesWritten];
            }

            _frameBuffer.AsSpan(0, _frame.BytesWritten).CopyTo(buffer);
            return _frame;
        }
    }

    public void Dispose()
    {
        lock (_captureLock)
        {
            if (_disposed) return;
            _disposed = true;
        }
        DisposeCore();
    }

    private void DisposeCore()
    {
        if (_framePool is not null)
        {
            _framePool.FrameArrived -= OnFrameArrived;
        }

        if (_captureItem is not null)
        {
            _captureItem.Closed -= OnCaptureClosed;
        }

        _session?.Dispose();
        _session = null;

        _framePool?.Dispose();
        _framePool = null;

        _stagingTexture?.Dispose();
        _stagingTexture = null;

        _winrtDevice?.Dispose();
        _d3dDevice?.Dispose();

        lock (_frameLock)
        {
            if (_frameBuffer is not null)
            {
                ArrayPool<byte>.Shared.Return(_frameBuffer);
                _frameBuffer = null;
            }
        }

        _frameEvent.Dispose();
    }

    private void OnCaptureClosed(GraphicsCaptureItem sender, object args)
    {
        lock (_captureLock)
        {
            if (_disposed) return;
            lock (_frameLock)
            {
                _fatalError = new InvalidOperationException("The captured window has closed.");
            }
            _frameEvent.Set();
        }
    }

    // Requests dirty regions for measurement only; the full frame is still
    // rendered and copied (ReportOnly). Returns the mode for the stats line.
    private string EnableDirtyRegions()
    {
        if (!DirtyRegionsPresent.Value)
        {
            return "unsupported";
        }
        try
        {
            _session!.DirtyRegionMode = GraphicsCaptureDirtyRegionMode.ReportOnly;
            _reportDirtyRegions = true;
            return "report-only";
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine($"[capture] dirty region mode unavailable: {ex.Message}");
            return "error";
        }
    }

    // Reads the frame's dirty regions, or null when they are unavailable. A
    // failure is logged once and treated as unknown, never as "no change".
    private IReadOnlyList<RectInt32>? ReadDirtyRegions(Direct3D11CaptureFrame frame)
    {
        if (!_reportDirtyRegions)
        {
            return null;
        }
        try
        {
            return frame.DirtyRegions.ToArray();
        }
        catch (Exception ex)
        {
            if (!_dirtyRegionErrorLogged)
            {
                _dirtyRegionErrorLogged = true;
                Console.Error.WriteLine($"[capture] dirty regions unavailable: {ex.Message}");
            }
            return null;
        }
    }

    private void InitializeDevices()
    {
        if (_d3dDevice is not null && _winrtDevice is not null)
        {
            return;
        }

        var hr = D3D11CreateDeviceNative(
            IntPtr.Zero,
            DriverType.Hardware,
            IntPtr.Zero,
            (uint)DeviceCreationFlags.BgraSupport,
            null,
            0,
            D3D11SdkVersion,
            out var devicePointer,
            out _,
            out var contextPointer);
        Marshal.ThrowExceptionForHR(hr);

        try
        {
            _d3dDevice = new ID3D11Device(devicePointer);
            _winrtDevice = GraphicsCaptureInterop.CreateWinRtDevice(_d3dDevice);
        }
        finally
        {
            if (contextPointer != IntPtr.Zero)
            {
                Marshal.Release(contextPointer);
            }
        }
    }

    private void OnFrameArrived(Direct3D11CaptureFramePool sender, object args)
    {
        lock (_captureLock)
        {
            if (!_disposed) ProcessFrame(sender);
        }
    }

    private void ProcessFrame(Direct3D11CaptureFramePool sender)
    {
        try
        {
            using var frame = sender.TryGetNextFrame();
            if (frame is null)
            {
                return;
            }

            if (frame.ContentSize.Width <= 0 || frame.ContentSize.Height <= 0)
            {
                return;
            }

            if (frame.ContentSize.Width != _captureWidth || frame.ContentSize.Height != _captureHeight)
            {
                _captureWidth = frame.ContentSize.Width;
                _captureHeight = frame.ContentSize.Height;
                sender.Recreate(
                    _winrtDevice!,
                    DirectXPixelFormat.B8G8R8A8UIntNormalized,
                    2,
                    frame.ContentSize);
                _stats?.MarkResized();
                return;
            }

            // Read before the frame is disposed; it describes this frame only.
            var dirtyRegions = _stats is null ? null : ReadDirtyRegions(frame);
            var copyStarted = Stopwatch.GetTimestamp();

            var surfaceInterop = frame.Surface.As<IDirect3DDxgiInterfaceAccess>();
            var resourceGuid = typeof(ID3D11Texture2D).GUID;
            var resourcePointer = surfaceInterop.GetInterface(ref resourceGuid);
            using var gpuTexture = new ID3D11Texture2D(resourcePointer);

            EnsureStagingTexture(gpuTexture.Description);
            _d3dDevice!.ImmediateContext.CopyResource(_stagingTexture!, gpuTexture);

            _d3dDevice.ImmediateContext.Map(_stagingTexture!, 0, MapMode.Read, Vortice.Direct3D11.MapFlags.None, out var mapped).CheckError();
            try
            {
                var copyWidth = Math.Min(frame.ContentSize.Width, (int)gpuTexture.Description.Width);
                var copyHeight = Math.Min(frame.ContentSize.Height, (int)gpuTexture.Description.Height);
                var maxWidthFromPitch = checked((int)mapped.RowPitch) / 4;
                if (copyWidth > maxWidthFromPitch)
                {
                    copyWidth = maxWidthFromPitch;
                }

                if (copyWidth <= 0 || copyHeight <= 0)
                {
                    return;
                }

                StoreFrame(mapped.DataPointer, copyWidth, copyHeight, checked((int)mapped.RowPitch), dirtyRegions, copyStarted);
            }
            finally
            {
                _d3dDevice.ImmediateContext.Unmap(_stagingTexture!, 0);
            }
        }
        catch (Exception ex)
        {
            lock (_frameLock)
            {
                _fatalError = ex;
            }

            _frameEvent.Set();
        }
    }

    private void EnsureStagingTexture(Texture2DDescription sourceDescription)
    {
        if (_stagingTexture is not null &&
            _stagingTexture.Description.Width == sourceDescription.Width &&
            _stagingTexture.Description.Height == sourceDescription.Height)
        {
            return;
        }

        _stagingTexture?.Dispose();
        var stagingDescription = new Texture2DDescription
        {
            Width = sourceDescription.Width,
            Height = sourceDescription.Height,
            MipLevels = 1,
            ArraySize = 1,
            Format = Vortice.DXGI.Format.B8G8R8A8_UNorm,
            SampleDescription = new Vortice.DXGI.SampleDescription(1, 0),
            Usage = ResourceUsage.Staging,
            BindFlags = BindFlags.None,
            CPUAccessFlags = CpuAccessFlags.Read,
            MiscFlags = ResourceOptionFlags.None
        };

        _stagingTexture = _d3dDevice!.CreateTexture2D(stagingDescription);
    }

    private void StoreFrame(IntPtr pixels, int width, int height, int sourcePitch, IReadOnlyList<RectInt32>? dirtyRegions, long copyStarted)
    {
        if (sourcePitch <= 0 || width <= 0 || height <= 0)
        {
            return;
        }

        var safeWidth = Math.Min(width, sourcePitch / 4);
        if (safeWidth <= 0)
        {
            return;
        }

        var contiguousStride = checked(safeWidth * 4);
        var requiredSize = checked(contiguousStride * height);

        lock (_frameLock)
        {
            if (_frameBuffer is null || _frameBuffer.Length < requiredSize)
            {
                var buffer = ArrayPool<byte>.Shared.Rent(requiredSize);
                if (_frameBuffer is not null)
                {
                    ArrayPool<byte>.Shared.Return(_frameBuffer);
                }

                _frameBuffer = buffer;
            }

            if (sourcePitch == contiguousStride)
            {
                Marshal.Copy(pixels, _frameBuffer, 0, requiredSize);
            }
            else
            {
                for (var y = 0; y < height; y++)
                {
                    Marshal.Copy(
                        IntPtr.Add(pixels, checked(y * sourcePitch)),
                        _frameBuffer,
                        y * contiguousStride,
                        contiguousStride);
                }
            }

            _frame = new FrameInfo(safeWidth, height, _frame.FrameId + 1);
            _stats?.RecordFrame(safeWidth, height, dirtyRegions, Stopwatch.GetTimestamp() - copyStarted,
                _frameBuffer.AsSpan(0, requiredSize));
        }

        _frameEvent.Set();
    }

    private void ThrowIfDisposed()
    {
        if (_disposed)
        {
            throw new ObjectDisposedException(nameof(WgcCaptureService));
        }
    }

    private void ThrowIfFaulted()
    {
        lock (_frameLock)
        {
            if (_fatalError is not null)
            {
                throw new InvalidOperationException("Capture session failed.", _fatalError);
            }
        }
    }

    [DllImport("d3d11.dll", EntryPoint = "D3D11CreateDevice", ExactSpelling = true)]
    private static extern int D3D11CreateDeviceNative(
        IntPtr adapter,
        DriverType driverType,
        IntPtr software,
        uint flags,
        [MarshalAs(UnmanagedType.LPArray, SizeParamIndex = 5)] FeatureLevel[]? featureLevels,
        uint featureLevelsCount,
        uint sdkVersion,
        out IntPtr device,
        out FeatureLevel featureLevel,
        out IntPtr immediateContext);

    private const uint D3D11SdkVersion = 7;
}

internal readonly record struct FrameInfo(int Width, int Height, long FrameId)
{
    public int Stride => checked(Width * 4);
    public int BytesWritten => checked(Stride * Height);
}

using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

namespace CaptureProbe;

internal sealed record WindowInfo(
    long Handle,
    string Title,
    int ProcessId,
    string ProcessName,
    string ClassName,
    string? IconPng);

internal static class WindowEnumeration
{
    public static IReadOnlyList<WindowInfo> ListVisibleWindows()
    {
        var windows = new List<WindowInfo>();
        EnumWindows((hwnd, _) =>
        {
            if (!IsWindowVisible(hwnd) || IsWindowCloaked(hwnd))
            {
                return true;
            }

            var length = GetWindowTextLengthW(hwnd);
            if (length <= 0)
            {
                return true;
            }

            var titleBuilder = new StringBuilder(length + 1);
            _ = GetWindowTextW(hwnd, titleBuilder, titleBuilder.Capacity);
            var title = titleBuilder.ToString().Trim();
            if (string.IsNullOrWhiteSpace(title))
            {
                return true;
            }

            GetWindowThreadProcessId(hwnd, out var processId);
            string processName;
            try
            {
                using var process = Process.GetProcessById((int)processId);
                processName = process.ProcessName;
            }
            catch
            {
                processName = "unknown";
            }

            var classBuilder = new StringBuilder(256);
            _ = GetClassNameW(hwnd, classBuilder, classBuilder.Capacity);
            windows.Add(new WindowInfo(
                (long)hwnd,
                title,
                unchecked((int)processId),
                processName,
                classBuilder.ToString(),
                WindowIcon.GetPng(hwnd)));

            return true;
        }, (nint)0);

        return windows;
    }

    private const int DwmWindowCloaked = 14;

    private static bool IsWindowCloaked(nint hwnd)
    {
        return DwmGetWindowAttribute(hwnd, DwmWindowCloaked, out var cloaked, sizeof(int)) == 0 && cloaked != 0;
    }

    private delegate bool EnumWindowsProc(nint hwnd, nint lParam);

    [DllImport("user32.dll")]
    private static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, nint lParam);

    [DllImport("user32.dll")]
    private static extern bool IsWindowVisible(nint hWnd);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetWindowTextLengthW(nint hWnd);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetWindowTextW(nint hWnd, StringBuilder lpString, int nMaxCount);

    [DllImport("user32.dll")]
    private static extern uint GetWindowThreadProcessId(nint hWnd, out uint lpdwProcessId);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetClassNameW(nint hWnd, StringBuilder lpClassName, int nMaxCount);

    [DllImport("dwmapi.dll")]
    private static extern int DwmGetWindowAttribute(nint hwnd, int dwAttribute, out int pvAttribute, int cbAttribute);
}

package win32

// dpiAwarenessPerMonitorV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4).
const dpiAwarenessPerMonitorV2 = ^uintptr(3)

// EnableDPIAwareness makes the process per-monitor DPI aware, so window
// rectangles are in the same physical pixels as Windows Graphics Capture
// frames, even across monitors with different scaling.
func EnableDPIAwareness() error {
	if ok, _, err := procSetProcessDpiAwarenessCtx.Call(dpiAwarenessPerMonitorV2); ok == 0 {
		return callError(err)
	}
	return nil
}

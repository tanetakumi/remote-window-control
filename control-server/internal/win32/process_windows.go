package win32

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

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

// HideConsole prevents console windows for cmd while preserving redirected I/O.
func HideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

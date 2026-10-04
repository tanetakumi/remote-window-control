package win32

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

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

// childJob kills its processes when the host exits, however it exits: the
// handle is never closed, so the kernel closes it when the host process ends.
var childJob = sync.OnceValues(func() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
})

// KillWithHost makes a started child process end together with the host, so
// a killed host cannot leave capture helpers (and their capture border) or
// encoders running. Only children are assigned: the host itself stays outside
// the job, so programs it opens through the shell are not tied to it.
func KillWithHost(process *os.Process) error {
	job, err := childJob()
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	// The *os.Process keeps the child's handle open, so its PID cannot be reused.
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("open process %d: %w", process.Pid, err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		return fmt.Errorf("assign process %d to job object: %w", process.Pid, err)
	}
	return nil
}

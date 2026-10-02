package tray

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"share-app-host/internal/config"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	registerClass       = user32.NewProc("RegisterClassExW")
	unregisterClass     = user32.NewProc("UnregisterClassW")
	createWindow        = user32.NewProc("CreateWindowExW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	postMessage         = user32.NewProc("PostMessageW")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	registerMessage     = user32.NewProc("RegisterWindowMessageW")
	createIcon          = user32.NewProc("CreateIconFromResourceEx")
	destroyIcon         = user32.NewProc("DestroyIcon")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	createMenu          = user32.NewProc("CreatePopupMenu")
	appendMenu          = user32.NewProc("AppendMenuW")
	destroyMenu         = user32.NewProc("DestroyMenu")
	trackMenu           = user32.NewProc("TrackPopupMenu")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	getCursorPos        = user32.NewProc("GetCursorPos")
	notifyIcon          = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmClose         = 0x0010
	wmContextMenu   = 0x007b
	wmTray          = 0x8001 // WM_APP + 1
	ninSelect       = 0x0400
	ninKeySelect    = 0x0401
	nimAdd          = 0
	nimDelete       = 2
	nimSetFocus     = 3
	nimSetVersion   = 4
	exitCommand     = 1
	openDataCommand = 2
)

type windowClass struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}

type point struct{ X, Y int32 }

type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}

type notifyData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

func callError(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("tray: %s failed", operation)
	}
	return fmt.Errorf("tray: %s: %w", operation, err)
}

// Start creates the icon on a dedicated Windows message thread. It reports
// initialization errors synchronously. onExit must return promptly (the host
// supplies a context cancel function). The returned cleanup waits for removal.
func Start(onExit func()) (func(), error) {
	onExit = sync.OnceFunc(onExit)
	ready := make(chan error, 1)
	done := make(chan struct{})
	var hwnd uintptr // Published by ready; only read by cleanup thereafter.
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		err := run(onExit, func(window uintptr) {
			hwnd = window
			ready <- nil
		})
		if hwnd == 0 {
			ready <- err
		} else {
			if err != nil {
				log.Printf("tray stopped: %v", err)
			}
			// An unexpected message-loop exit must not leave an invisible host.
			onExit()
		}
	}()
	if err := <-ready; err != nil {
		<-done
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			select {
			case <-done:
				return
			default:
			}
			if ok, _, err := postMessage.Call(hwnd, wmClose, 0, 0); ok == 0 {
				log.Printf("%v", callError("post close", err))
				return
			}
			<-done
		})
	}, nil
}

func run(onExit func(), ready func(uintptr)) error {
	var instance windows.Handle
	err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance)
	if err != nil {
		return fmt.Errorf("tray: module handle: %w", err)
	}
	className := windows.StringToUTF16Ptr("ShareAppTray")
	taskbarCreated, _, err := registerMessage.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("TaskbarCreated"))))
	if taskbarCreated == 0 {
		return callError("register taskbar message", err)
	}
	var data notifyData
	callback := syscall.NewCallback(func(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case wmTray:
			switch uint16(lParam) {
			case wmContextMenu, ninSelect, ninKeySelect:
				if err := showMenu(hwnd, &data, onExit); err != nil {
					log.Print(err)
				}
			}
			return 0
		case wmClose:
			postQuitMessage.Call(0)
			return 0
		case uint32(taskbarCreated):
			// Use a hidden top-level window, rather than a message-only window,
			// so Explorer's restart broadcast reaches us. Windows also sends it
			// on taskbar DPI changes while our icon still exists, so remove any
			// stale entry first or NIM_ADD would fail and exit the host.
			notifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
			if err := addIcon(&data); err != nil {
				log.Print(err)
				onExit()
			}
			return 0
		}
		result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return result
	})
	class := windowClass{WndProc: callback, Instance: uintptr(instance), ClassName: className}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return callError("register window class", err)
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(className)), uintptr(instance))
	hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		return callError("create window", err)
	}
	defer destroyWindow.Call(hwnd)
	size, _, _ := getSystemMetrics.Call(49) // SM_CXSMICON, scaled for host DPI.
	icon, _, err := createIcon.Call(uintptr(unsafe.Pointer(&iconPNG[0])), uintptr(len(iconPNG)), 1, 0x00030000, size, size, 0)
	if icon == 0 {
		return callError("create icon", err)
	}
	defer destroyIcon.Call(icon)
	data = notifyData{Window: hwnd, ID: 1, Flags: 0x01 | 0x02 | 0x04 | 0x80, Callback: wmTray, Icon: icon, Version: 4}
	data.Size = uint32(unsafe.Sizeof(data))
	copy(data.Tip[:], windows.StringToUTF16("Share App — 起動中"))
	if err := addIcon(&data); err != nil {
		return err
	}
	defer notifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	ready(hwnd)
	var msg message
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return callError("get message", err)
		}
		if result == 0 {
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func addIcon(data *notifyData) error {
	if ok, _, err := notifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(data))); ok == 0 {
		return callError("add icon", err)
	}
	if ok, _, err := notifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(data))); ok == 0 {
		notifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(data)))
		return callError("set icon version", err)
	}
	return nil
}

func showMenu(hwnd uintptr, data *notifyData, onExit func()) error {
	menu, _, err := createMenu.Call()
	if menu == 0 {
		return callError("create menu", err)
	}
	defer destroyMenu.Call(menu)
	openLabel := windows.StringToUTF16Ptr("Open data folder（データフォルダーを開く）")
	if ok, _, err := appendMenu.Call(menu, 0, openDataCommand, uintptr(unsafe.Pointer(openLabel))); ok == 0 {
		return callError("append open data folder menu", err)
	}
	label := windows.StringToUTF16Ptr("Exit（終了）")
	if ok, _, err := appendMenu.Call(menu, 0, exitCommand, uintptr(unsafe.Pointer(label))); ok == 0 {
		return callError("append exit menu", err)
	}
	var position point
	if ok, _, err := getCursorPos.Call(uintptr(unsafe.Pointer(&position))); ok == 0 {
		return callError("get menu position", err)
	}
	// Foreground ownership and the trailing WM_NULL let an outside click dismiss
	// the menu. TPM_RETURNCMD avoids a second, asynchronous command handler.
	setForegroundWindow.Call(hwnd)
	command, _, _ := trackMenu.Call(menu, 0x0100|0x0002, uintptr(position.X), uintptr(position.Y), 0, hwnd, 0)
	postMessage.Call(hwnd, 0, 0, 0)
	notifyIcon.Call(nimSetFocus, uintptr(unsafe.Pointer(data)))
	switch command {
	case openDataCommand:
		return openDataFolder(hwnd)
	case exitCommand:
		log.Print("exit requested from tray")
		onExit()
	}
	return nil
}

func openDataFolder(hwnd uintptr) error {
	dir, err := config.UserDataDir()
	if err != nil {
		return fmt.Errorf("tray: open data folder: %w", err)
	}
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return fmt.Errorf("tray: data folder path: %w", err)
	}
	if err := windows.ShellExecute(windows.Handle(hwnd), windows.StringToUTF16Ptr("open"), path, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("tray: open data folder %q: %w", dir, err)
	}
	return nil
}

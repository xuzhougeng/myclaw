//go:build windows

package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmCommand            = 0x0111
	wmContextMenu        = 0x007B
	wmNull               = 0x0000
	wmLButtonUp          = 0x0202
	wmLButtonDblClk      = 0x0203
	wmRButtonUp          = 0x0205
	trayCallbackMessage  = 0x8001
	gwlpWndProc          = -4
	gwOwner              = 4
	mfString             = 0x00000000
	mfSeparator          = 0x00000800
	nimAdd               = 0x00000000
	nimDelete            = 0x00000002
	nifMessage           = 0x00000001
	nifIcon              = 0x00000002
	nifTip               = 0x00000004
	tpmLeftAlign         = 0x0000
	tpmBottomAlign       = 0x0020
	tpmRightButton       = 0x0002
	idiApplication       = 32512
	trayIconID           = 1
	trayMenuShowCommand  = 0xF001
	trayMenuExitCommand  = 0xF002
	trayWindowClass      = "wailsWindow"
	trayInitTimeout      = 5 * time.Second
	trayInitPollInterval = 50 * time.Millisecond
)

type point struct {
	X int32
	Y int32
}

type notifyIconData struct {
	CbSize            uint32
	HWnd              windows.Handle
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             windows.Handle
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          windows.GUID
	HBalloonIcon      windows.Handle
}

type windowsTrayController struct {
	app             *DesktopApp
	hwnd            windows.Handle
	menu            windows.Handle
	icon            windows.Handle
	ownsIcon        bool
	originalWndProc uintptr

	mu       sync.Mutex
	disposed bool
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procCallWindowProcW     = user32.NewProc("CallWindowProcW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procExtractIconW        = shell32.NewProc("ExtractIconW")
	procGetClassNameW       = user32.NewProc("GetClassNameW")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetWindow           = user32.NewProc("GetWindow")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procIsWindow            = user32.NewProc("IsWindow")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")

	trayWindowProcCallback = syscall.NewCallback(windowsTrayWindowProc)
	trayControllers        sync.Map
)

func newDesktopTrayController(app *DesktopApp) (desktopTrayController, error) {
	hwnd, err := waitForMainWindow(trayInitTimeout)
	if err != nil {
		return nil, err
	}

	controller := &windowsTrayController{
		app:  app,
		hwnd: hwnd,
	}
	if err := controller.install(); err != nil {
		return nil, err
	}
	return controller, nil
}

func (c *windowsTrayController) Dispose() error {
	c.mu.Lock()
	if c.disposed {
		c.mu.Unlock()
		return nil
	}
	c.disposed = true
	hwnd := c.hwnd
	menu := c.menu
	icon := c.icon
	ownsIcon := c.ownsIcon
	originalWndProc := c.originalWndProc
	c.menu = 0
	c.icon = 0
	c.ownsIcon = false
	c.originalWndProc = 0
	c.mu.Unlock()

	var disposeErr error
	if err := deleteTrayIcon(hwnd); err != nil {
		disposeErr = err
	}

	if isWindow(hwnd) && originalWndProc != 0 {
		if _, err := setWindowLongPtr(hwnd, gwlpWndProc, originalWndProc); err != nil && disposeErr == nil {
			disposeErr = err
		}
	}
	trayControllers.Delete(uintptr(hwnd))

	if menu != 0 {
		procDestroyMenu.Call(uintptr(menu))
	}
	if ownsIcon && icon != 0 {
		procDestroyIcon.Call(uintptr(icon))
	}

	return disposeErr
}

func (c *windowsTrayController) install() error {
	menuHandle, err := createTrayMenu()
	if err != nil {
		return err
	}
	c.menu = menuHandle

	iconHandle, ownsIcon, err := loadTrayIcon()
	if err != nil {
		procDestroyMenu.Call(uintptr(menuHandle))
		c.menu = 0
		return err
	}
	c.icon = iconHandle
	c.ownsIcon = ownsIcon

	originalWndProc, err := setWindowLongPtr(c.hwnd, gwlpWndProc, trayWindowProcCallback)
	if err != nil {
		c.cleanupHandles()
		return err
	}
	c.originalWndProc = originalWndProc
	trayControllers.Store(uintptr(c.hwnd), c)

	if err := addTrayIcon(c.hwnd, c.icon); err != nil {
		trayControllers.Delete(uintptr(c.hwnd))
		_, _ = setWindowLongPtr(c.hwnd, gwlpWndProc, originalWndProc)
		c.cleanupHandles()
		return err
	}
	return nil
}

func (c *windowsTrayController) cleanupHandles() {
	if c.menu != 0 {
		procDestroyMenu.Call(uintptr(c.menu))
		c.menu = 0
	}
	if c.ownsIcon && c.icon != 0 {
		procDestroyIcon.Call(uintptr(c.icon))
	}
	c.icon = 0
	c.ownsIcon = false
}

func (c *windowsTrayController) handleWindowMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case trayCallbackMessage:
		switch lParam {
		case wmLButtonUp, wmLButtonDblClk:
			go c.app.restoreMainWindow()
			return 0
		case wmRButtonUp, wmContextMenu:
			c.showTrayMenu()
			return 0
		}
	case wmCommand:
		switch uint16(wParam & 0xffff) {
		case trayMenuShowCommand:
			go c.app.restoreMainWindow()
			return 0
		case trayMenuExitCommand:
			go c.app.quitFromTray()
			return 0
		}
	}

	return callWindowProc(c.originalWndProc, hwnd, msg, wParam, lParam)
}

func (c *windowsTrayController) showTrayMenu() {
	c.mu.Lock()
	if c.disposed || c.menu == 0 {
		c.mu.Unlock()
		return
	}
	menuHandle := c.menu
	hwnd := c.hwnd
	c.mu.Unlock()

	var cursor point
	if result, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); result == 0 {
		return
	}

	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(
		uintptr(menuHandle),
		uintptr(tpmLeftAlign|tpmBottomAlign|tpmRightButton),
		uintptr(cursor.X),
		uintptr(cursor.Y),
		0,
		uintptr(hwnd),
		0,
	)
	procPostMessageW.Call(uintptr(hwnd), wmNull, 0, 0)
}

func waitForMainWindow(timeout time.Duration) (windows.Handle, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		hwnd, err := findMainWindow()
		if err == nil {
			return hwnd, nil
		}
		lastErr = err
		time.Sleep(trayInitPollInterval)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("main window not found")
	}
	return 0, lastErr
}

func findMainWindow() (windows.Handle, error) {
	pid := uint32(os.Getpid())
	var firstMatch windows.Handle
	var wailsMatch windows.Handle

	enumProc := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		windowHandle := windows.Handle(hwnd)
		if !belongsToCurrentProcess(windowHandle, pid) {
			return 1
		}
		if isOwnedWindow(windowHandle) {
			return 1
		}
		if firstMatch == 0 {
			firstMatch = windowHandle
		}
		if windowClassName(windowHandle) == trayWindowClass {
			wailsMatch = windowHandle
			return 0
		}
		return 1
	})

	procEnumWindows.Call(enumProc, 0)

	switch {
	case wailsMatch != 0:
		return wailsMatch, nil
	case firstMatch != 0:
		return firstMatch, nil
	default:
		return 0, fmt.Errorf("wails window not found")
	}
}

func belongsToCurrentProcess(hwnd windows.Handle, pid uint32) bool {
	var windowPID uint32
	procGetWindowThreadPID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&windowPID)))
	return windowPID == pid
}

func isOwnedWindow(hwnd windows.Handle) bool {
	owner, _, _ := procGetWindow.Call(uintptr(hwnd), uintptr(gwOwner))
	return owner != 0
}

func windowClassName(hwnd windows.Handle) string {
	var buffer [64]uint16
	result, _, _ := procGetClassNameW.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if result == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer[:result])
}

func createTrayMenu() (windows.Handle, error) {
	result, _, err := procCreatePopupMenu.Call()
	if result == 0 {
		return 0, fmt.Errorf("CreatePopupMenu: %w", err)
	}
	menuHandle := windows.Handle(result)

	if err := appendMenuString(menuHandle, trayMenuShowCommand, "Show baize"); err != nil {
		procDestroyMenu.Call(result)
		return 0, err
	}
	appendResult, _, appendErr := procAppendMenuW.Call(uintptr(menuHandle), mfSeparator, 0, 0)
	if appendResult == 0 {
		procDestroyMenu.Call(result)
		return 0, fmt.Errorf("AppendMenuW separator: %w", appendErr)
	}
	if err := appendMenuString(menuHandle, trayMenuExitCommand, "Exit"); err != nil {
		procDestroyMenu.Call(result)
		return 0, err
	}

	return menuHandle, nil
}

func appendMenuString(menuHandle windows.Handle, commandID uintptr, label string) error {
	labelPtr, err := windows.UTF16PtrFromString(label)
	if err != nil {
		return err
	}
	result, _, callErr := procAppendMenuW.Call(
		uintptr(menuHandle),
		mfString,
		commandID,
		uintptr(unsafe.Pointer(labelPtr)),
	)
	if result == 0 {
		return fmt.Errorf("AppendMenuW %q: %w", label, callErr)
	}
	return nil
}

func loadTrayIcon() (windows.Handle, bool, error) {
	if exePath, err := os.Executable(); err == nil {
		pathPtr, ptrErr := windows.UTF16PtrFromString(exePath)
		if ptrErr == nil {
			result, _, _ := procExtractIconW.Call(0, uintptr(unsafe.Pointer(pathPtr)), 0)
			if result > 1 {
				return windows.Handle(result), true, nil
			}
		}
	}

	result, _, err := procLoadIconW.Call(0, idiApplication)
	if result == 0 {
		return 0, false, fmt.Errorf("LoadIconW: %w", err)
	}
	return windows.Handle(result), false, nil
}

func addTrayIcon(hwnd, icon windows.Handle) error {
	data := notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             hwnd,
		UID:              trayIconID,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: trayCallbackMessage,
		HIcon:            icon,
	}
	copy(data.SzTip[:], windows.StringToUTF16("baize"))

	return shellNotifyIcon(nimAdd, &data)
}

func deleteTrayIcon(hwnd windows.Handle) error {
	data := notifyIconData{
		CbSize: uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:   hwnd,
		UID:    trayIconID,
	}
	return shellNotifyIcon(nimDelete, &data)
}

func shellNotifyIcon(action uint32, data *notifyIconData) error {
	result, _, err := procShellNotifyIconW.Call(uintptr(action), uintptr(unsafe.Pointer(data)))
	if result == 0 {
		return fmt.Errorf("Shell_NotifyIconW action %d: %w", action, err)
	}
	return nil
}

func setWindowLongPtr(hwnd windows.Handle, index int, value uintptr) (uintptr, error) {
	result, _, err := procSetWindowLongPtrW.Call(uintptr(hwnd), uintptr(index), value)
	if result == 0 {
		return 0, fmt.Errorf("SetWindowLongPtrW: %w", err)
	}
	return result, nil
}

func isWindow(hwnd windows.Handle) bool {
	result, _, _ := procIsWindow.Call(uintptr(hwnd))
	return result != 0
}

func windowsTrayWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	controllerValue, ok := trayControllers.Load(hwnd)
	if !ok {
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return result
	}
	return controllerValue.(*windowsTrayController).handleWindowMessage(hwnd, msg, wParam, lParam)
}

func callWindowProc(previous uintptr, hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if previous == 0 {
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return result
	}
	result, _, _ := procCallWindowProcW.Call(previous, hwnd, uintptr(msg), wParam, lParam)
	return result
}

//go:build windows

package native

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/brielshallouf/syncamp/internal/platform"
	"github.com/brielshallouf/syncamp/internal/svcclient"
	"github.com/brielshallouf/syncamp/internal/webui"
)

const (
	popW, popH, popMargin = 400, 680, 12 // device-independent pixels

	swHide, swShow = 0, 5

	wsPopup       = 0x80000000
	wsCaption     = 0x00C00000
	wsThickFrame  = 0x00040000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000
	wsExToolWin   = 0x00000080
	wsExAppWindow = 0x00040000

	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	dwmwaCornerPreference = 33
	dwmwcpRound           = 2
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	dwmapi          = windows.NewLazySystemDLL("dwmapi.dll")
	pGetWindowLong  = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLong  = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos   = user32.NewProc("SetWindowPos")
	pShowWindow     = user32.NewProc("ShowWindow")
	pSetForeground  = user32.NewProc("SetForegroundWindow")
	pDwmSetAttr     = dwmapi.NewProc("DwmSetWindowAttribute")
	pDwmExtendFrame = dwmapi.NewProc("DwmExtendFrameIntoClientArea")

	gwlStyle   = ^uintptr(15) // -16
	gwlExStyle = ^uintptr(19) // -20
	hwndTop    = ^uintptr(0)  // HWND_TOPMOST (-1)
)

// ErrUnsupported is unused on Windows; kept so callers compile on every OS.
var ErrUnsupported = errors.New("no native window on this platform")

// Popover is the app's own window: frameless, always on top, placed beside the taskbar
// (whichever edge it is docked to), showing the UI inside the app with no browser, no
// address bar and no web server. It is created once and then hidden/shown; if the user
// closes it (Alt+F4) the next click recreates it.
type Popover struct {
	// OnReport, if set, receives the UI's one-time "rendered" report (used by the CI self-test).
	OnReport func(string)

	client *svcclient.Client

	createMu sync.Mutex
	mu       sync.Mutex
	wv       webview2.WebView
	hwnd     uintptr
	visible  bool
	lastHide time.Time
}

// New builds a popover bound to the service client. Nothing is created until Warm/Show.
func New(c *svcclient.Client) *Popover { return &Popover{client: c} }

// Available reports whether the window currently exists.
func (p *Popover) Available() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.wv != nil }

// Warm creates the window ahead of the first click, so opening it is instant.
func (p *Popover) Warm() { go func() { _ = p.ensure() }() }

func (p *Popover) ensure() error {
	p.createMu.Lock()
	defer p.createMu.Unlock()
	p.mu.Lock()
	have := p.wv != nil
	p.mu.Unlock()
	if have {
		return nil
	}
	ready := make(chan error, 1)
	go p.loop(ready)
	select {
	case err := <-ready:
		return err
	case <-time.After(45 * time.Second):
		return errors.New("timed out starting the SyncAmp window")
	}
}

// loop owns the window: it must create it, run its message loop and destroy it on one OS thread.
func (p *Popover) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// WebView2 is a COM component and COM is per-thread. The library initialises COM only on the
	// process's MAIN thread (in its package init), but this window lives on its own thread: without
	// this call the environment-created callback never arrives and startup hangs forever.
	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); {
	case err == nil:
		defer windows.CoUninitialize()
	case err == windows.Errno(1): // S_FALSE: this thread was already initialised, which is fine
		defer windows.CoUninitialize()
	default:
		logf("CoInitializeEx failed: %v", err)
	}
	logf("creating the window")
	data := filepath.Join(os.Getenv("LOCALAPPDATA"), "SyncAmp", "WebView2") // not Program Files: not writable
	_ = os.MkdirAll(data, 0o700)
	// Created tiny so the unavoidable first-paint flash is a 1-pixel window; it is hidden at once.
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true, DataPath: data,
		WindowOptions: webview2.WindowOptions{Title: "SyncAmp", Width: 1, Height: 1},
	})
	if wv == nil {
		logf("WebView2 could not be created (see the lines above)")
		ready <- errors.New("the Microsoft WebView2 Runtime is not installed. Install it from https://go.microsoft.com/fwlink/p/?LinkId=2124703 and open SyncAmp again")
		return
	}
	hwnd := uintptr(wv.Window())
	logf("window created (hwnd %x)", hwnd)
	frameless(hwnd)
	_, _, _ = pShowWindow.Call(hwnd, swHide)

	// The page's only way out. start() hands the request to a goroutine and returns at once:
	// bound functions run on this UI thread, so a slow service must never block here.
	_ = wv.Bind("syncampStart", func(id int, method, path, body string) {
		go func() {
			r := p.client.Do(method, path, body)
			b, _ := json.Marshal(r.Body)
			wv.Dispatch(func() { wv.Eval(fmt.Sprintf("window.syncampDone(%d,%d,%s)", id, r.Status, b)) })
		}()
	})
	_ = wv.Bind("syncampHide", func() { p.Hide() })
	_ = wv.Bind("syncampReport", func(s string) {
		if p.OnReport != nil {
			go p.OnReport(s)
		}
	})
	wv.SetHtml(webui.Inline())

	p.mu.Lock()
	p.wv, p.hwnd = wv, hwnd
	p.mu.Unlock()
	ready <- nil

	wv.Run()

	p.mu.Lock()
	p.wv, p.hwnd, p.visible = nil, 0, false
	p.mu.Unlock()
	wv.Destroy()
}

// frameless removes the title bar and borders, hides the taskbar button, and (Windows 11)
// rounds the corners. A 1px extended frame keeps the system drop shadow.
func frameless(hwnd uintptr) {
	style, _, _ := pGetWindowLong.Call(hwnd, gwlStyle)
	style &^= wsCaption | wsThickFrame | wsSysMenu | wsMinimizeBox | wsMaximizeBox
	style |= wsPopup
	_, _, _ = pSetWindowLong.Call(hwnd, gwlStyle, style)
	ex, _, _ := pGetWindowLong.Call(hwnd, gwlExStyle)
	ex = (ex &^ wsExAppWindow) | wsExToolWin
	_, _, _ = pSetWindowLong.Call(hwnd, gwlExStyle, ex)
	pref := uint32(dwmwcpRound)
	_, _, _ = pDwmSetAttr.Call(hwnd, dwmwaCornerPreference, uintptr(unsafe.Pointer(&pref)), unsafe.Sizeof(pref))
	m := struct{ L, R, T, B int32 }{1, 1, 1, 1}
	_, _, _ = pDwmExtendFrame.Call(hwnd, uintptr(unsafe.Pointer(&m)))
	_, _, _ = pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x0002|0x0001|swpFrameChanged|swpNoActivate|0x0004) // NOMOVE|NOSIZE|NOZORDER
}

// Show places the window beside the taskbar and brings it to the front.
func (p *Popover) Show() error {
	if err := p.ensure(); err != nil {
		return err
	}
	p.mu.Lock()
	wv, hwnd := p.wv, p.hwnd
	p.mu.Unlock()
	if wv == nil {
		return errors.New("window is gone")
	}
	wv.Dispatch(func() {
		scale := platform.DPIScale(hwnd)
		w, h, m := int(popW*scale), int(popH*scale), int(popMargin*scale)
		x, y := 100, 100
		if screen, work, ok := platform.ScreenAndWorkArea(); ok {
			x, y, h, _ = platform.PlacePanel(screen, work, w, h, m)
		}
		_, _, _ = pSetWindowPos.Call(hwnd, hwndTop, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpFrameChanged|swpNoActivate)
		_, _, _ = pShowWindow.Call(hwnd, swShow)
		_, _, _ = pSetForeground.Call(hwnd)
		p.mu.Lock()
		p.visible = true
		p.mu.Unlock()
	})
	return nil
}

// Hide hides the window (it stays loaded).
func (p *Popover) Hide() {
	p.mu.Lock()
	wv, hwnd := p.wv, p.hwnd
	p.mu.Unlock()
	if wv == nil {
		return
	}
	wv.Dispatch(func() {
		_, _, _ = pShowWindow.Call(hwnd, swHide)
		p.mu.Lock()
		p.visible, p.lastHide = false, time.Now()
		p.mu.Unlock()
	})
}

// Toggle shows the window, or hides it if it is already open. A click on the tray icon first
// takes focus from the popover (which hides it); that same click must not reopen it.
func (p *Popover) Toggle() error {
	p.mu.Lock()
	vis, since := p.visible, time.Since(p.lastHide)
	p.mu.Unlock()
	switch {
	case vis:
		p.Hide()
		return nil
	case since < 400*time.Millisecond:
		return nil
	}
	return p.Show()
}

var logOnce sync.Once

// logf appends to %LOCALAPPDATA%\SyncAmp\app.log. The app has no console, and the WebView2 library
// reports its errors through the standard logger, so without this a failure would be invisible.
func logf(format string, a ...any) {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "SyncAmp")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "app.log")
	flags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if st, err := os.Stat(path); err == nil && st.Size() > 256<<10 {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY // keep it small
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	logOnce.Do(func() { log.SetOutput(f) }) // route the library's own messages here as well
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
	if log.Writer() != f {
		f.Close()
	}
}

// Notify shows a simple message box (used for errors such as a missing WebView2 runtime).
func Notify(title, message string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(message)
	_, _ = windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONINFORMATION)
}

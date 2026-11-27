//go:build !darwin

package tray

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/brielshallouf/syncamp/internal/native"
	"github.com/brielshallouf/syncamp/internal/svcclient"
)

// Run shows the notification-area icon and blocks until the user quits. On Windows,
// clicking the icon opens the app's own window (pop); there is no browser and no URL
// anywhere. On other systems the icon offers a native menu.
func Run(client *svcclient.Client, pop *native.Popover) {
	systray.Run(func() { onReady(client, pop) }, func() {})
}

func onReady(client *svcclient.Client, pop *native.Popover) {
	systray.SetIcon(iconFor("unknown"))
	systray.SetTitle("SyncAmp")
	systray.SetTooltip("SyncAmp: connecting…")

	open := func() {
		if err := pop.Toggle(); err != nil && !errors.Is(err, native.ErrUnsupported) {
			native.Notify("SyncAmp", err.Error())
		}
	}
	hasWindow := runtime.GOOS == "windows"
	if hasWindow {
		pop.Warm() // create the window in the background so the first click is instant
		systray.SetOnTapped(open)
	}

	mStatus := systray.AddMenuItem("Connecting…", "Current backup status")
	mStatus.Disable()
	systray.AddSeparator()
	var mOpen *systray.MenuItem
	if hasWindow {
		mOpen = systray.AddMenuItem("Open SyncAmp", "")
	}
	mRun := systray.AddMenuItem("Back up all now", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit SyncAmp", "The background service keeps running")

	poll := func() {
		r := client.Do("GET", "/api/summary", "")
		var st struct{ Overall, Summary string }
		if r.Status != 200 || json.Unmarshal([]byte(r.Body), &st) != nil {
			systray.SetIcon(iconFor("unknown"))
			systray.SetTooltip("SyncAmp: the background service is not running")
			mStatus.SetTitle("Service not running")
			return
		}
		systray.SetIcon(iconFor(st.Overall))
		tip := fmt.Sprintf("SyncAmp: %s", st.Summary)
		systray.SetTooltip(tip)
		mStatus.SetTitle(tip)
		if client.Token() == "" {
			mRun.SetTitle("Back up all now (needs access: run `syncamp grant <you>`)")
			mRun.Disable()
		} else {
			mRun.SetTitle("Back up all now")
			mRun.Enable()
		}
	}
	go func() {
		poll()
		for range time.Tick(5 * time.Second) {
			poll()
		}
	}()

	openCh := (<-chan struct{})(nil)
	if mOpen != nil {
		openCh = mOpen.ClickedCh
	}
	go func() {
		for {
			select {
			case <-openCh:
				open()
			case <-mRun.ClickedCh:
				client.Do("POST", "/api/run", "")
				poll()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

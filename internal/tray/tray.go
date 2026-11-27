package tray

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fyne.io/systray"

	"github.com/brielshallouf/syncamp/internal/platform"
)

type status struct {
	Overall string `json:"overall"`
	Summary string `json:"summary"`
}

// Run shows the tray icon and blocks until the user quits. base is the
// service API root, e.g. http://127.0.0.1:5801.
func Run(base string) {
	systray.Run(func() { onReady(base) }, func() {})
}

func onReady(base string) {
	systray.SetIcon(iconFor("unknown"))
	systray.SetTitle("SyncAmp")
	systray.SetTooltip("SyncAmp: connecting…")
	mStatus := systray.AddMenuItem("Connecting…", "Current backup status")
	mStatus.Disable()
	systray.AddSeparator()
	mOpen := systray.AddMenuItem("Open dashboard", "")
	mRun := systray.AddMenuItem("Run all backups now", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit tray", "The SyncAmp service keeps running")

	client := &http.Client{Timeout: 5 * time.Second}
	post := func(path string) {
		req, _ := http.NewRequest(http.MethodPost, base+path, nil)
		req.Header.Set("X-SyncAmp", "1")
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	poll := func() {
		resp, err := client.Get(base + "/api/status")
		if err != nil {
			systray.SetIcon(iconFor("unknown"))
			systray.SetTooltip("SyncAmp service not reachable")
			mStatus.SetTitle("Service not reachable")
			return
		}
		defer resp.Body.Close()
		var st status
		if json.NewDecoder(resp.Body).Decode(&st) != nil {
			return
		}
		systray.SetIcon(iconFor(st.Overall))
		tip := fmt.Sprintf("SyncAmp: %s", st.Summary)
		systray.SetTooltip(tip)
		mStatus.SetTitle(tip)
	}
	go func() {
		poll()
		t := time.NewTicker(5 * time.Second)
		for range t.C {
			poll()
		}
	}()
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				_ = platform.OpenBrowser(base + "/")
			case <-mRun.ClickedCh:
				post("/api/run")
				poll()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

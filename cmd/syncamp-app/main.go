//go:build !darwin

// Command syncamp-app is the SyncAmp desktop app: a notification-area icon whose click
// opens the app's own window (Windows), or a native menu (Linux). It talks to the
// background service through a small in-process bridge: no browser, no URL, no sign-in.
// Headless servers don't need it: everything is available through `syncamp`.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/brielshallouf/syncamp/internal/autostart"
	"github.com/brielshallouf/syncamp/internal/native"
	"github.com/brielshallouf/syncamp/internal/platform"
	"github.com/brielshallouf/syncamp/internal/svcclient"
	"github.com/brielshallouf/syncamp/internal/tray"
)

func main() {
	platform.EnableDPIAwareness() // before any window exists, or Windows blurs us on hi-res screens
	args := os.Args[1:]
	client := svcclient.New(platform.DataDir())
	switch {
	case len(args) == 2 && args[0] == "autostart":
		exe, _ := os.Executable()
		var err error
		switch args[1] {
		case "on":
			err = autostart.Enable(exe)
		case "off":
			err = autostart.Disable()
		default:
			err = fmt.Errorf("usage: syncamp-app [autostart on|off]")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Println("ok")
	case len(args) == 2 && args[0] == "selftest":
		os.Exit(selftest(client, args[1]))
	case len(args) == 0:
		tray.Run(client, native.New(client))
	default:
		fmt.Fprintln(os.Stderr, "usage: syncamp-app [autostart on|off | selftest OUTFILE]")
		os.Exit(2)
	}
}

// selftest opens the real window, waits for the UI to render the live service state, and
// writes what it saw to out. Used by CI to prove, on a real Windows machine, that the
// native window, the page and the Go bridge all work. Exit: 0 ok, 1 timeout, 2 not signed
// in, 3 window could not start.
func selftest(c *svcclient.Client, out string) int {
	pop := native.New(c)
	got := make(chan string, 1)
	pop.OnReport = func(s string) {
		select {
		case got <- s:
		default:
		}
	}
	write := func(m map[string]any) {
		b, _ := json.MarshalIndent(m, "", "  ")
		_ = os.WriteFile(out, b, 0o644)
	}
	if err := pop.Show(); err != nil {
		write(map[string]any{"error": err.Error()})
		return 3
	}
	select {
	case s := <-got:
		var rep map[string]any
		_ = json.Unmarshal([]byte(s), &rep)
		write(map[string]any{"report": rep, "tokenReadable": c.Token() != "", "addr": c.Addr()})
		if rep["signedIn"] != true {
			return 2
		}
		return 0
	case <-time.After(60 * time.Second):
		write(map[string]any{"error": "timed out waiting for the UI to render", "tokenReadable": c.Token() != ""})
		return 1
	}
}

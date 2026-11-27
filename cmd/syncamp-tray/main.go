// Command syncamp-tray shows the SyncAmp status icon for the logged-in user.
// It is a separate binary so the privileged service links no GUI code.
// Headless servers don't need it: everything is available via `syncamp`.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/brielshallouf/syncamp/internal/auth"
	"github.com/brielshallouf/syncamp/internal/autostart"
	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/platform"
	"github.com/brielshallouf/syncamp/internal/tray"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "autostart" {
		exe, _ := os.Executable()
		var err error
		switch os.Args[2] {
		case "on":
			err = autostart.Enable(exe)
		case "off":
			err = autostart.Disable()
		default:
			err = fmt.Errorf("usage: syncamp-tray [autostart on|off]")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Println("ok")
		return
	}
	cfg, _ := config.Load(platform.ConfigPath()) // unreadable for non-admins => defaults
	// An admin-run tray can read the token and offer "Run all"; a normal
	// user's tray only gets the read-only summary.
	tok, _ := auth.Read(filepath.Dir(platform.ConfigPath()))
	tray.Run("http://"+cfg.ListenAddr, tok)
}

// Command syncamp is a cross-platform (Windows/Linux) supervisor for the
// Syncrify backup client: runs as an OS service, exposes a local dashboard,
// and ships a system-tray front end.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kardianos/service"

	"github.com/brielshallouf/syncamp/internal/api"
	"github.com/brielshallouf/syncamp/internal/autostart"
	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/core"
	"github.com/brielshallouf/syncamp/internal/platform"
	"github.com/brielshallouf/syncamp/internal/tray"
)

var version = "dev"

const usage = `syncamp - service + tray for Syncrify backups

Usage: syncamp [-config FILE] <command>

  run                 run in the foreground (this is what the service executes)
  install             install as a service (Windows service / systemd unit); needs admin/root
  uninstall           remove the service
  start|stop|restart  control the installed service
  status              print current status from the running service
  profiles            list discovered Syncrify profiles
  backup [NAME]       trigger a backup now (all profiles if NAME omitted)
  tray                show the system-tray icon (run as the logged-in user)
  tray-autostart on|off   start the tray at login
  init                write a default config file
  version
`

type program struct {
	cfgPath string
	cancel  context.CancelFunc
	done    chan struct{}
}

func (p *program) Start(service.Service) error {
	e, err := core.New(p.cfgPath)
	if err != nil {
		return err
	}
	cfg, _ := config.Load(p.cfgPath)
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	e.Start(ctx)
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: api.Handler(e), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		defer close(p.done)
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Printf("api: %v", err)
			}
		}()
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(sctx)
		e.Wait()
	}()
	log.Printf("SyncAmp %s listening on http://%s (config %s)", version, cfg.ListenAddr, p.cfgPath)
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(15 * time.Second):
		}
	}
	return nil
}

func main() {
	cfgPath := flag.String("config", platform.ConfigPath(), "config file")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	cmd := flag.Arg(0)
	if cmd == "" {
		flag.Usage()
		os.Exit(2)
	}

	svcCfg := &service.Config{
		Name: "SyncAmp", DisplayName: "SyncAmp Backup Supervisor",
		Description: "Schedules, monitors and verifies Syncrify backups.",
		Arguments:   []string{"-config", *cfgPath, "run"},
	}
	prg := &program{cfgPath: *cfgPath}
	svc, err := service.New(prg, svcCfg)
	if err != nil {
		fatal(err)
	}

	switch cmd {
	case "run":
		if !service.Interactive() { // under SCM/systemd: log to a file
			_ = os.MkdirAll(filepath.Dir(*cfgPath), 0o755)
			if f, err := os.OpenFile(filepath.Join(filepath.Dir(*cfgPath), "syncamp.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				log.SetOutput(f)
			}
		}
		fatal(svc.Run())
	case "install", "uninstall", "start", "stop", "restart":
		fatal(service.Control(svc, cmd))
		fmt.Println("ok:", cmd)
		if cmd == "install" {
			fmt.Println("Next: `syncamp start`, then `syncamp tray-autostart on` as the logged-in user.")
		}
	case "status":
		var st core.Status
		get(apiBase(*cfgPath)+"/api/status", &st)
		fmt.Printf("%s: %s\nclient: %s\n", st.Overall, st.Summary, st.Client)
		for _, p := range st.Profiles {
			last := "never run"
			if p.Last != nil {
				last = p.Last.Status + " @ " + p.Last.End.Format(time.DateTime)
			}
			fmt.Printf("  %-24s %s\n", p.Name, last)
		}
	case "profiles":
		cfg, _ := config.Load(*cfgPath)
		fmt.Println("client:", orNone(cfg.ClientPath))
		e, err := core.New(*cfgPath)
		fatal(err)
		for _, p := range e.Profiles() {
			fmt.Printf("  %-24s %s\n", p.Name, p.Path)
		}
	case "backup":
		path := "/api/run"
		if n := flag.Arg(1); n != "" {
			path += "/" + n
		}
		req, _ := http.NewRequest(http.MethodPost, apiBase(*cfgPath)+path, nil)
		req.Header.Set("X-SyncAmp", "1")
		resp, err := http.DefaultClient.Do(req)
		fatal(err)
		b, _ := io.ReadAll(resp.Body)
		fmt.Println(string(b))
	case "tray":
		tray.Run(apiBase(*cfgPath))
	case "tray-autostart":
		exe, _ := os.Executable()
		switch flag.Arg(1) {
		case "on":
			fatal(autostart.Enable(exe))
		case "off":
			fatal(autostart.Disable())
		default:
			flag.Usage()
			os.Exit(2)
		}
		fmt.Println("ok")
	case "init":
		if _, err := os.Stat(*cfgPath); err == nil {
			fatal(fmt.Errorf("%s already exists", *cfgPath))
		}
		fatal(config.Save(*cfgPath, config.Default()))
		fmt.Println("wrote", *cfgPath)
	case "version":
		fmt.Println(version)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func apiBase(cfgPath string) string {
	cfg, _ := config.Load(cfgPath)
	return "http://" + cfg.ListenAddr
}

func get(url string, v any) {
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(url)
	if err != nil {
		fatal(fmt.Errorf("service not reachable (%v) - is it running?", err))
	}
	defer resp.Body.Close()
	fatal(json.NewDecoder(resp.Body).Decode(v))
}

func orNone(s string) string {
	if s == "" {
		return "(not found - set clientPath in config)"
	}
	return s
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

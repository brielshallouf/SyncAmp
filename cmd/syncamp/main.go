// Command syncamp is a cross-platform (Windows/Linux) supervisor for the
// Syncrify backup client: it runs as an OS service and is fully operable from
// the command line on headless machines. The tray UI is a separate binary
// (syncamp-tray) so the privileged service carries no GUI code.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kardianos/service"

	"github.com/brielshallouf/syncamp/internal/api"
	"github.com/brielshallouf/syncamp/internal/auth"
	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/core"
	"github.com/brielshallouf/syncamp/internal/logrotate"
	"github.com/brielshallouf/syncamp/internal/platform"
)

var version = "dev"

const usage = `syncamp - supervisor for Syncrify backups (service + CLI)

Usage: syncamp [-config FILE] <command> [args]

Service
  install | uninstall      install/remove the service (needs Administrator/root)
  start | stop | restart   control the installed service
  run                      run in the foreground (what the service executes)

Operate (talk to the running service; need Administrator/root to read the token)
  status [-json]           health summary; exit 0=ok 1=warn 2=error/unreachable
  profiles                 list discovered Syncrify profiles
  backup [-wait] [NAME]    start a backup now (all profiles if NAME omitted)
  history [-n N]           recent runs
  logs NAME                captured output of the profile's last run
  doctor [-json]           security & configuration audit (no service needed)
  token                    print the API token (for the web dashboard)

Configure
  init                     write a default config
  config show|validate|set KEY VALUE
  profile enable|disable|reset NAME
  profile schedule NAME [-days mon,wed] [-times 02:00] [-interval 6h]

  version
`

type program struct {
	cfgPath string
	cancel  context.CancelFunc
	done    chan struct{}
}

func (p *program) Start(service.Service) error {
	dir := filepath.Dir(p.cfgPath)
	if err := platform.SecureDir(dir); err != nil {
		return fmt.Errorf("secure data dir: %w", err)
	}
	// Repair loose permissions left by manual edits, restores or old versions.
	for _, f := range []string{p.cfgPath, filepath.Join(dir, auth.FileName), filepath.Join(dir, "history.json"), filepath.Join(dir, "syncamp.log")} {
		if _, err := os.Stat(f); err == nil {
			_ = platform.SecureFile(f)
		}
	}
	// Fail closed: an invalid config (e.g. non-loopback listener) stops startup.
	e, err := core.New(p.cfgPath)
	if err != nil {
		return err
	}
	token, err := auth.LoadOrCreate(dir)
	if err != nil {
		return fmt.Errorf("api token: %w", err)
	}
	cfg := e.Config()
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	e.Start(ctx)
	srv := &http.Server{
		Addr: cfg.ListenAddr, Handler: api.Handler(e, token),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
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

func main() { os.Exit(run()) }

func run() int {
	cfgPath := flag.String("config", platform.ConfigPath(), "config file")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	cmd, rest := flag.Arg(0), flag.Args()
	if cmd == "" {
		flag.Usage()
		return 2
	}
	rest = rest[1:]

	svcCfg := &service.Config{
		Name: "SyncAmp", DisplayName: "SyncAmp Backup Supervisor",
		Description: "Schedules, monitors and verifies Syncrify backups.",
		Arguments:   []string{"-config", *cfgPath, "run"},
		Option:      service.KeyValue{"SystemdScript": hardenedUnit},
	}
	svc, err := service.New(&program{cfgPath: *cfgPath}, svcCfg)
	if err != nil {
		return fail(err)
	}

	switch cmd {
	case "run":
		if !service.Interactive() { // under SCM/systemd: size-capped file log
			dir := filepath.Dir(*cfgPath)
			if platform.SecureDir(dir) == nil {
				log.SetOutput(&logrotate.Writer{Path: filepath.Join(dir, "syncamp.log"), Max: 5 << 20})
			}
		}
		return fail(svc.Run())
	case "install":
		dir := filepath.Dir(*cfgPath)
		if err := platform.SecureDir(dir); err != nil {
			return fail(err)
		}
		if _, err := auth.LoadOrCreate(dir); err != nil {
			return fail(err)
		}
		if err := service.Control(svc, "install"); err != nil {
			return fail(err)
		}
		fmt.Println("installed. Next: syncamp doctor && syncamp start && syncamp status")
		return 0
	case "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, cmd); err != nil {
			return fail(err)
		}
		fmt.Println("ok:", cmd)
		return 0
	case "status":
		return cmdStatus(*cfgPath, rest)
	case "doctor":
		return cmdDoctor(*cfgPath, rest)
	case "backup":
		return cmdBackup(*cfgPath, rest)
	case "history":
		return cmdHistory(*cfgPath, rest)
	case "logs":
		return cmdLogs(*cfgPath, rest)
	case "token":
		return cmdToken(*cfgPath)
	case "config":
		return cmdConfig(*cfgPath, rest)
	case "profile":
		return cmdProfile(*cfgPath, rest)
	case "profiles":
		e, err := core.New(*cfgPath)
		if err != nil {
			return fail(err)
		}
		fmt.Println("client:", orNone(e.Config().ClientPath))
		for _, p := range e.Profiles() {
			fmt.Printf("  %-24s %s\n", p.Name, p.Path)
		}
		return 0
	case "init":
		if _, err := os.Stat(*cfgPath); err == nil {
			return fail(fmt.Errorf("%s already exists", *cfgPath))
		}
		if err := config.Save(*cfgPath, config.Default()); err != nil {
			return fail(err)
		}
		fmt.Println("wrote", *cfgPath)
		return 0
	case "version":
		fmt.Println(version)
		return 0
	}
	flag.Usage()
	return 2
}

func fail(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

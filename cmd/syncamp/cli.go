package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brielshallouf/syncamp/internal/audit"
	"github.com/brielshallouf/syncamp/internal/auth"
	"github.com/brielshallouf/syncamp/internal/config"
	"github.com/brielshallouf/syncamp/internal/core"
	"github.com/brielshallouf/syncamp/internal/store"
)

// client talks to the running service's API on behalf of the CLI.
type client struct {
	base, token string
	http        *http.Client
}

func newClient(cfgPath string) (*client, error) {
	cfg, _ := config.Load(cfgPath) // listenAddr falls back to default if unreadable
	tok, err := auth.Read(filepath.Dir(cfgPath))
	if err != nil {
		return nil, fmt.Errorf("cannot read API token (%v): run as Administrator/root", err)
	}
	return &client{base: "http://" + cfg.ListenAddr, token: tok, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (c *client) do(method, path string, out any) error {
	req, _ := http.NewRequest(method, c.base+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("service not reachable (%v) - is it running? try: syncamp start", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// cmdStatus exit codes follow monitoring-plugin convention: 0 ok, 1 warn, 2 error/unreachable.
func cmdStatus(cfgPath string, args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	fs.Parse(args)
	c, err := newClient(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	var st core.Status
	if err := c.do("GET", "/api/status", &st); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	if *asJSON {
		b, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Printf("%s: %s\nclient: %s\n", strings.ToUpper(st.Overall), st.Summary, orNone(st.Client))
		for _, p := range st.Profiles {
			last := "never run"
			if p.Running {
				last = "running"
			} else if p.Last != nil {
				last = p.Last.Status + " @ " + p.Last.End.Format(time.DateTime)
			}
			fmt.Printf("  %-24s %s\n", p.Name, last)
		}
		for _, f := range st.Findings {
			fmt.Printf("  [%s] %s\n", f.Level, f.Msg)
		}
		for _, k := range st.Skipped {
			fmt.Printf("  [WARN] skipped %s: %s\n", k.Path, k.Reason)
		}
	}
	switch st.Overall {
	case "ok", "running", "idle":
		return 0
	case "warn":
		return 1
	}
	return 2
}

func cmdDoctor(cfgPath string, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	fs.Parse(args)
	cfg, err := config.Load(cfgPath)
	findings := audit.Check(cfgPath, cfg)
	if err != nil && !hasMsg(findings, "config invalid") {
		findings = append([]audit.Finding{{Level: audit.Fail, Msg: "config: " + err.Error()}}, findings...)
	}
	rc := 0
	for _, f := range findings {
		if f.Level == audit.Fail {
			rc = 2
		} else if f.Level == audit.Warn && rc == 0 {
			rc = 1
		}
	}
	if *asJSON {
		b, _ := json.MarshalIndent(findings, "", "  ")
		fmt.Println(string(b))
		return rc
	}
	for _, f := range findings {
		fmt.Printf("[%-4s] %s\n", f.Level, f.Msg)
	}
	if rc == 0 {
		fmt.Println("no problems found")
	}
	return rc
}

func hasMsg(fs []audit.Finding, prefix string) bool {
	for _, f := range fs {
		if strings.HasPrefix(f.Msg, prefix) {
			return true
		}
	}
	return false
}

func cmdBackup(cfgPath string, args []string) int {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	wait := fs.Bool("wait", false, "block until finished; exit non-zero if the backup failed")
	fs.Parse(args)
	c, err := newClient(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	name := fs.Arg(0)
	path := "/api/run"
	if name != "" {
		path += "/" + url.PathEscape(name)
	}
	startedAt := time.Now().Add(-2 * time.Second)
	if err := c.do("POST", path, nil); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println("started")
	if !*wait {
		return 0
	}
	for {
		time.Sleep(2 * time.Second)
		var st core.Status
		if err := c.do("GET", "/api/status", &st); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 2
		}
		busy, bad := false, false
		for _, p := range st.Profiles {
			if name != "" && p.Name != name {
				continue
			}
			if p.Running {
				busy = true
			} else if p.Last != nil && p.Last.Start.After(startedAt) && p.Last.Status != "success" {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", p.Name, p.Last.Status, p.Last.Error)
				if p.Last.Status != "warning" {
					bad = true // warnings are reported but do not fail scripts
				}
			}
		}
		if !busy {
			if bad {
				return 1
			}
			fmt.Println("finished")
			return 0
		}
	}
}

func cmdHistory(cfgPath string, args []string) int {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	n := fs.Int("n", 20, "number of runs")
	fs.Parse(args)
	c, err := newClient(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	var runs []store.Run
	if err := c.do("GET", "/api/history", &runs); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	if *n < len(runs) {
		runs = runs[:*n]
	}
	for _, r := range runs {
		fmt.Printf("%s  %-20s %-8s %-8s %s\n", r.Start.Format(time.DateTime), r.Profile, r.Status, r.Trigger, r.Error)
	}
	return 0
}

// cmdLogs prints the captured (redacted) client output of a profile's last run.
func cmdLogs(cfgPath string, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: syncamp logs PROFILE")
		return 2
	}
	c, err := newClient(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	var runs []store.Run
	if err := c.do("GET", "/api/history", &runs); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	for _, r := range runs {
		if r.Profile == args[0] {
			fmt.Printf("# %s  %s  %s\n%s\n", r.Start.Format(time.DateTime), r.Status, r.Error, strings.Join(r.Tail, "\n"))
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "no runs recorded for", args[0])
	return 1
}

func cmdToken(cfgPath string) int {
	t, err := auth.Read(filepath.Dir(cfgPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err, "(start the service once to create it, and run as Administrator/root)")
		return 1
	}
	fmt.Println(t)
	return 0
}

// cmdConfig: show | validate | set KEY VALUE
func cmdConfig(cfgPath string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: syncamp config show|validate|set KEY VALUE")
		return 2
	}
	switch args[0] {
	case "show":
		cfg, err := config.LoadRaw(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if cfg.Webhook != "" { // webhook URLs usually embed a secret token
			if u, err := url.Parse(cfg.Webhook); err == nil {
				cfg.Webhook = u.Scheme + "://" + u.Host + "/…(redacted)"
			}
		}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(b))
	case "validate":
		if _, err := config.Load(cfgPath); err != nil {
			fmt.Fprintln(os.Stderr, "invalid:", err)
			return 1
		}
		fmt.Println("config is valid")
	case "set":
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: syncamp config set KEY VALUE\nkeys: webhook, server.url, server.checkInterval, clientPath, jobTimeout, mode (monitor|scheduler), staleAfter (auto|DURATION), logWatch (on|off), defaultTimes (comma separated)")
			return 2
		}
		cfg, err := config.LoadRaw(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		k, v := args[1], args[2]
		switch k {
		case "webhook":
			cfg.Webhook = v
		case "server.url":
			cfg.Server.URL = v
		case "server.checkInterval":
			cfg.Server.CheckInterval = v
		case "clientPath":
			cfg.ClientPath = v
		case "jobTimeout":
			cfg.JobTimeout = v
		case "mode":
			cfg.Mode = v
		case "staleAfter":
			cfg.StaleAfter = v
		case "logWatch":
			cfg.LogWatch = v == "on" || v == "true"
		case "defaultTimes":
			cfg.DefaultTimes = splitList(v)
		default:
			fmt.Fprintln(os.Stderr, "unknown or non-settable key", k, "(security-sensitive options must be edited in the file)")
			return 2
		}
		return saveAndReload(cfgPath, cfg)
	default:
		fmt.Fprintln(os.Stderr, "unknown config subcommand", args[0])
		return 2
	}
	return 0
}

// cmdProfile: enable|disable NAME  |  schedule NAME [-days a,b] [-times HH:MM,..] [-interval 6h] | reset NAME
func cmdProfile(cfgPath string, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: syncamp profile enable|disable|reset NAME\n       syncamp profile stale NAME auto|26h\n       syncamp profile schedule NAME [-days mon,wed] [-times 02:00,14:30] [-interval 6h]")
		return 2
	}
	sub, name := args[0], args[1]
	cfg, err := config.LoadRaw(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Override{}
	}
	ov := cfg.Profiles[name]
	switch sub {
	case "enable":
		t := true
		ov.Enabled = &t
	case "disable":
		f := false
		ov.Enabled = &f
	case "reset":
		delete(cfg.Profiles, name)
		return saveAndReload(cfgPath, cfg)
	case "stale":
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: syncamp profile stale NAME auto|DURATION   (alert if no successful backup within DURATION, e.g. 26h)")
			return 2
		}
		if args[2] == "auto" {
			ov.StaleAfter = ""
		} else {
			ov.StaleAfter = args[2]
		}
	case "schedule":
		fs := flag.NewFlagSet("schedule", flag.ExitOnError)
		days := fs.String("days", "", "comma-separated weekdays, e.g. mon,wed,fri")
		times := fs.String("times", "", "comma-separated HH:MM, e.g. 02:00,14:30")
		interval := fs.String("interval", "", "run every duration, e.g. 6h (replaces days/times)")
		fs.Parse(args[2:])
		if *interval != "" && (*days != "" || *times != "") {
			fmt.Fprintln(os.Stderr, "use either -interval or -days/-times")
			return 2
		}
		if *days == "" && *times == "" && *interval == "" {
			fmt.Fprintln(os.Stderr, "nothing to change")
			return 2
		}
		if *interval != "" {
			ov.Interval, ov.Days, ov.Times = *interval, nil, nil
		} else {
			ov.Interval = ""
			if *days != "" {
				ov.Days = splitList(*days)
			}
			if *times != "" {
				ov.Times = splitList(*times)
			}
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown profile subcommand", sub)
		return 2
	}
	cfg.Profiles[name] = ov
	return saveAndReload(cfgPath, cfg)
}

// cmdWatch adds/removes a folder of Syncrify profiles that SyncAmp should look at.
func cmdWatch(cfgPath, cmd string, args []string) int {
	if len(args) != 1 || !filepath.IsAbs(args[0]) {
		fmt.Fprintf(os.Stderr, "usage: syncamp %s FULL_PATH_TO_PROFILE_FOLDER\n", cmd)
		return 2
	}
	dir := filepath.Clean(args[0])
	cfg, err := config.LoadRaw(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	var kept []string
	have := false
	for _, d := range cfg.ProfileDirs {
		if strings.EqualFold(filepath.Clean(d), dir) {
			have = true
			if cmd == "unwatch" {
				continue
			}
		}
		kept = append(kept, d)
	}
	if cmd == "watch" && !have {
		kept = append(kept, dir)
		fmt.Println("Note: Syncrify profiles can run scripts, and SyncAmp starts backups with the service's rights.\n" +
			"Only watch a folder that nobody you don't trust can write to.")
	}
	cfg.ProfileDirs = kept
	return saveAndReload(cfgPath, cfg)
}

func saveAndReload(cfgPath string, cfg config.Config) int {
	if err := config.Save(cfgPath, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println("saved", cfgPath)
	if c, err := newClient(cfgPath); err == nil {
		if err := c.do("POST", "/api/reload", nil); err == nil {
			fmt.Println("service reloaded")
		} else {
			fmt.Println("(service not running; changes apply on next start)")
		}
	}
	return 0
}

func splitList(s string) []string {
	var out []string
	for _, p := range bytes.Split([]byte(s), []byte(",")) {
		if t := strings.TrimSpace(string(p)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(not found - set clientPath)"
	}
	return s
}

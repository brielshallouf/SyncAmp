package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectsUnsafe(t *testing.T) {
	bad := map[string]func(*Config){
		"public listener":     func(c *Config) { c.ListenAddr = "0.0.0.0:5801" },
		"lan listener":        func(c *Config) { c.ListenAddr = "192.168.1.5:5801" },
		"hostname listener":   func(c *Config) { c.ListenAddr = "example.com:5801" },
		"relative client":     func(c *Config) { c.ClientPath = "SyncrifyClient.sh" },
		"bad regex":           func(c *Config) { c.FailurePatterns = []string{"("} },
		"bad time":            func(c *Config) { c.DefaultTimes = []string{"25:00"} },
		"http webhook":        func(c *Config) { c.Webhook = "http://hooks.example.com/x" },
		"file webhook":        func(c *Config) { c.Webhook = "file:///etc/passwd" },
		"tiny interval":       func(c *Config) { c.Profiles = map[string]Override{"a": {Interval: "1s"}} },
		"non-http server url": func(c *Config) { c.Server.URL = "ftp://x" },
	}
	for name, mut := range bad {
		c := Default()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	good := Default()
	good.Webhook = "https://hooks.example.com/x"
	good.ListenAddr = "[::1]:5801"
	if err := good.Validate(); err != nil {
		t.Errorf("good config rejected: %v", err)
	}
	c := Default()
	c.Webhook, c.AllowInsecureWebhook = "http://10.0.0.5/x", true
	if err := c.Validate(); err != nil {
		t.Errorf("explicit insecure webhook override should pass: %v", err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	// A typo like "allowInsecureWebook" in a security option must not be silently ignored.
	_ = os.WriteFile(p, []byte(`{"listenAdr":"0.0.0.0:1"}`), 0o600)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("want unknown-field error, got %v", err)
	}
}

func TestSaveIsOwnerOnlyAndRoundTrips(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "c.json")
	c := Default()
	c.Webhook = "https://hooks.example.com/secret"
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm()&0o077 != 0 {
		t.Errorf("config mode %v exposes secrets", st.Mode().Perm())
	}
	if st, _ := os.Stat(filepath.Dir(p)); st.Mode().Perm()&0o077 != 0 {
		t.Errorf("config dir mode %v", st.Mode().Perm())
	}
	got, err := LoadRaw(p)
	if err != nil || got.Webhook != c.Webhook {
		t.Errorf("roundtrip: %v %v", got.Webhook, err)
	}
	bad := Default()
	bad.ListenAddr = "0.0.0.0:1"
	if Save(p, bad) == nil {
		t.Error("Save must refuse invalid config")
	}
}

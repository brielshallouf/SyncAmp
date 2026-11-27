package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestInlineIsSelfContainedAndPinnedByHash(t *testing.T) {
	h := Inline()
	if strings.Contains(h, `href="/style.css"`) || strings.Contains(h, `src="/app.js"`) {
		t.Fatal("still references external files")
	}
	csp := regexp.MustCompile(`Content-Security-Policy" content="([^"]+)"`).FindStringSubmatch(h)
	if csp == nil {
		t.Fatal("no CSP")
	}
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "connect-src", "*"} {
		if strings.Contains(csp[1], bad) {
			t.Errorf("CSP must not contain %q: %s", bad, csp[1])
		}
	}
	for tag, directive := range map[string]string{"style": "style-src", "script": "script-src"} {
		m := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`).FindStringSubmatch(h)
		if m == nil {
			t.Fatalf("no inline <%s>", tag)
		}
		sum := sha256.Sum256([]byte(m[1]))
		want := directive + " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !strings.Contains(csp[1], want) {
			t.Errorf("CSP does not pin the inline %s by hash (want %q)", tag, want)
		}
	}
	if strings.Contains(h, "onclick") || strings.Contains(h, "javascript:") {
		t.Error("inline handlers are not allowed")
	}
}

func TestEmbeddedFilesExist(t *testing.T) {
	for _, f := range []string{"index.html", "app.js", "style.css"} {
		if b, err := File(f); err != nil || len(b) == 0 {
			t.Errorf("%s: %v", f, err)
		}
	}
}

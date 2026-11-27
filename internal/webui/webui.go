// Package webui holds the user interface (HTML/CSS/JS). It is served by the
// service for browser/SSH access and is also embedded, as one self-contained
// document, in the desktop app's native window.
package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"strings"
)

//go:embed web
var fs embed.FS

// File returns one embedded UI file (index.html, app.js, style.css).
func File(name string) ([]byte, error) { return fs.ReadFile("web/" + name) }

func must(name string) string {
	b, err := File(name)
	if err != nil {
		panic(err) // the files are embedded at build time
	}
	return string(b)
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// Inline returns the whole UI as ONE html document with the CSS and JS inlined,
// for display in the native window (no web server, no URL). Inline code is
// pinned by a content hash in a Content-Security-Policy, so nothing that was
// injected at run time could execute, and the page cannot make network
// requests or navigate: its only channel is the app's bound functions.
func Inline() string {
	css, js := must("style.css"), must("app.js")
	csp := "default-src 'none'; style-src " + hash(css) + "; script-src " + hash(js) +
		"; base-uri 'none'; form-action 'none'"
	html := must("index.html")
	html = strings.Replace(html, `<link rel="stylesheet" href="/style.css">`, `<style>`+css+`</style>`, 1)
	html = strings.Replace(html, `<script src="/app.js"></script>`, `<script>`+js+`</script>`, 1)
	return strings.Replace(html, "<head>", `<head><meta http-equiv="Content-Security-Policy" content="`+csp+`">`, 1)
}

//go:build !windows

// Package native is the desktop app's own window. On Windows it is a frameless popover
// next to the taskbar built on the system's WebView2. Other platforms have no
// native window from this Go code (macOS has a SwiftUI app; Linux uses the tray menu).
package native

import (
	"errors"

	"github.com/brielshallouf/syncamp/internal/svcclient"
)

// ErrUnsupported means this OS has no native popover window.
var ErrUnsupported = errors.New("no native window on this platform")

// Popover is the app window.
type Popover struct{ OnReport func(string) }

// New builds a popover bound to the service client.
func New(*svcclient.Client) *Popover { return &Popover{} }

// Warm, Show, Hide and Toggle are no-ops here.
func (p *Popover) Warm()           {}
func (p *Popover) Show() error     { return ErrUnsupported }
func (p *Popover) Hide()           {}
func (p *Popover) Toggle() error   { return ErrUnsupported }
func (p *Popover) Available() bool { return false }
func Notify(title, message string) {}

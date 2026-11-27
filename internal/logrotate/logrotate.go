// Package logrotate is a minimal size-capped log file writer, so an
// unattended server's disk can't be filled by the service log.
package logrotate

import (
	"os"
	"sync"
)

// Writer appends to Path; when it exceeds Max bytes it moves to Path+".1"
// (replacing any older backup) and starts afresh. Files are created 0600.
type Writer struct {
	Path string
	Max  int64

	mu   sync.Mutex
	f    *os.File
	size int64
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.f, w.size = f, st.Size()
	return nil
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size+int64(len(p)) > w.Max {
		w.f.Close()
		w.f = nil
		_ = os.Rename(w.Path, w.Path+".1")
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

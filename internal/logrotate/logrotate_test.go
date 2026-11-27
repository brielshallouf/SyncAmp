package logrotate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotationCapsDiskUse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "l.log")
	w := &Writer{Path: p, Max: 100}
	for i := 0; i < 50; i++ {
		if _, err := w.Write([]byte(strings.Repeat("x", 30) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	files, _ := filepath.Glob(p + "*")
	for _, f := range files {
		st, _ := os.Stat(f)
		total += st.Size()
	}
	if len(files) != 2 || total > 200 {
		t.Errorf("files=%v total=%d; want <=2 files, <=200 bytes", files, total)
	}
	if st, _ := os.Stat(p); st.Mode().Perm()&0o077 != 0 {
		t.Errorf("log mode %v", st.Mode().Perm())
	}
}

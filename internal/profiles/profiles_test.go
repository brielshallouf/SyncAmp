package profiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaysFromBitmap(t *testing.T) {
	// Documented example: Sun+Tue+Thu = 0010101b = 21.
	got := DaysFromBitmap(21)
	want := []time.Weekday{time.Sunday, time.Tuesday, time.Thursday}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v", got)
	}
}

func TestParseAndDiscover(t *testing.T) {
	dir := t.TempDir()
	xml := `<?xml version="1.0"?><profile><name>Docs</name><schedulerDays>21</schedulerDays>
	<encryptionKey>topsecret</encryptionKey><server><url>http://x:5800</url><password>pw</password></server></profile>`
	if err := os.WriteFile(filepath.Join(dir, "Docs.syncrify"), []byte(xml), 0o600); err != nil {
		t.Fatal(err)
	}
	ps := Discover([]string{dir, dir, "/nonexistent"})
	if len(ps) != 1 {
		t.Fatalf("want 1 profile (deduped), got %d", len(ps))
	}
	p := ps[0]
	if p.Name != "Docs" || len(p.Days) != 3 {
		t.Errorf("bad parse: %+v", p)
	}
	if p.Fields["encryptionKey"] != "***" || p.Fields["password"] != "***" {
		t.Errorf("secrets leaked: %v", p.Fields)
	}
}

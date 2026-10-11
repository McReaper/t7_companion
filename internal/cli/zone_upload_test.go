package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/zone"
)

// A fresh link is worth advising once the .xpak data earlier links replaced is
// 1% of the .xpak bytes.
func TestUploadAdvice(t *testing.T) {
	u := toUploadOut(zone.Upload{Bytes: 5 << 20, Xpak: 4 << 20, XpakDead: 0xa3d8}) // 1.0% of 4 MiB
	if u.Bytes != 5<<20 || u.XpakDead != 0xa3d8 || !strings.Contains(u.Advice, "at least 0.0 MB") || !strings.Contains(u.Advice, "fresh_xpak") {
		t.Errorf("%+v", u)
	}
	if u := toUploadOut(zone.Upload{Bytes: 5 << 20, Xpak: 4 << 20, XpakDead: 0xa3d6}); u.Advice != "" {
		t.Errorf("under 1%%: %+v", u)
	}
	if u := toUploadOut(zone.Upload{Bytes: 100}); u.Advice != "" || u.XpakDead != 0 {
		t.Errorf("no .xpak: %+v", u)
	}
	if u := upload(filepath.Join(t.TempDir(), "missing")); u != nil {
		t.Errorf("no zone/ folder: %+v", u)
	}
	if u := upload(t.TempDir()); u == nil || u.Bytes != 0 {
		t.Errorf("an empty zone/ folder: %+v", u)
	}
}

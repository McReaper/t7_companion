package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// update-check against a fake GitHub API: the verdict the setup skill relies on.
func TestUpdateCheck(t *testing.T) {
	status, tag := http.StatusOK, "v2.1.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub rejects API calls without a User-Agent")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://example.invalid/r"}`))
	}))
	defer srv.Close()
	oldURL, oldVersion := releaseAPIURL, version
	defer func() { releaseAPIURL, version = oldURL, oldVersion }()
	releaseAPIURL = srv.URL

	run := func() (string, error) {
		var out bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&out)
		err := runUpdateCheck(cmd)
		return out.String(), err
	}
	for _, c := range []struct{ version, want string }{
		{"2.1.0", "Up to date"},  // ldflags version has no "v", the tag does
		{"v2.1.0", "Up to date"}, // …or both do
		{"2.0.0", "Update available: 2.0.0 -> v2.1.0"},
		{"dev", "local/dev build"},
	} {
		version = c.version
		out, err := run()
		if err != nil || !strings.Contains(out, c.want) {
			t.Errorf("version %s: got %q (%v), want %q", c.version, out, err, c.want)
		}
	}
	status = http.StatusForbidden // rate-limited
	if _, err := run(); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("an API error must be reported, not read as up to date: %v", err)
	}
}

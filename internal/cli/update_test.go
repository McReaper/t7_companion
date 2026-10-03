package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
	oldURL, oldVersion, oldList := releaseAPIURL, version, listPlugins
	defer func() { releaseAPIURL, version, listPlugins = oldURL, oldVersion, oldList }()
	releaseAPIURL = srv.URL
	listPlugins = func() ([]byte, error) { return nil, exec.ErrNotFound } // the binary verdict alone

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

// update-check names each outdated plugin install with the commands that update
// it, and stays quiet when Claude Code isn't there to ask.
func TestUpdateCheckPlugins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.3.0","html_url":"https://example.invalid/r"}`))
	}))
	defer srv.Close()
	oldURL, oldVersion, oldList := releaseAPIURL, version, listPlugins
	defer func() { releaseAPIURL, version, listPlugins = oldURL, oldVersion, oldList }()
	releaseAPIURL, version = srv.URL, "2.3.0"

	run := func() string {
		var out bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&out)
		if err := runUpdateCheck(cmd); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	listPlugins = func() ([]byte, error) {
		return []byte(`[
			{"id":"t7kb@t7-reapy","version":"1.8.0","scope":"project","projectPath":"D:/BO3"},
			{"id":"t7kb@t7-reapy","version":"1.8.0","scope":"user"},
			{"id":"t7kb@t7-reapy","version":"2.3.0","scope":"local","projectPath":"D:/BO3"},
			{"id":"other@t7-reapy","version":"0.1.0","scope":"user"}
		]`), nil
	}
	out := run()
	for _, want := range []string{
		"Up to date (v2.3.0)",
		// beside a user-scope install, the project and local ones are removed, not updated
		"claude plugin uninstall t7kb@t7-reapy --scope project   (run from D:/BO3)",
		"claude plugin uninstall t7kb@t7-reapy --scope local   (run from D:/BO3)",
		"t7kb@t7-reapy 1.8.0 (user scope)",
		"claude plugin marketplace update t7-reapy\n",
		"claude plugin update t7kb@t7-reapy --scope user\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "update t7kb@t7-reapy --scope project") || strings.Contains(out, "other@") {
		t.Errorf("a scope to remove must not be updated, another plugin not listed:\n%s", out)
	}

	// no user-scope install: the project one is the install, so it is updated
	listPlugins = func() ([]byte, error) {
		return []byte(`[{"id":"t7kb@t7-reapy","version":"2.2.0","scope":"project","projectPath":"D:/BO3"}]`), nil
	}
	out = run()
	if !strings.Contains(out, "claude plugin update t7kb@t7-reapy --scope project   (run from D:/BO3)") || strings.Contains(out, "uninstall") {
		t.Errorf("a lone project install is updated, not removed:\n%s", out)
	}

	listPlugins = func() ([]byte, error) { return nil, exec.ErrNotFound }
	if out := run(); strings.Contains(out, "plugin") {
		t.Errorf("without Claude Code the plugin check stays quiet:\n%s", out)
	}
}

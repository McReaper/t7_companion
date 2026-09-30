package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// releaseAPIURL is a var so tests can point it at a local server.
var releaseAPIURL = "https://api.github.com/repos/McReaper/t7_companion/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func newUpdateCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update-check",
		Short: "Check GitHub for a newer t7kb release",
		Long: "Reports whether a newer t7kb release exists; it never downloads anything itself.\n" +
			"Re-run the installer with -Force/--force to actually update (see the README or the\n" +
			"setup skill) — this command only checks.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateCheck(cmd)
		},
	}
}

func runUpdateCheck(cmd *cobra.Command) error {
	rel, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("couldn't check for updates: %w", err)
	}

	out := cmd.OutOrStdout()
	current := strings.TrimPrefix(Version(), "v")
	latest := strings.TrimPrefix(rel.TagName, "v")

	switch {
	case Version() == "dev":
		fmt.Fprintf(out, "Running a local/dev build. Latest release: %s (%s)\n", rel.TagName, rel.HTMLURL)
	case current == latest:
		fmt.Fprintf(out, "Up to date (%s).\n", rel.TagName)
	default:
		fmt.Fprintf(out, "Update available: %s -> %s\n%s\n\n", Version(), rel.TagName, rel.HTMLURL)
		fmt.Fprintf(out, "Re-run the installer with -Force/--force to fetch the new binary + database\n"+
			"(a new release may bundle an updated database, uploaded separately). This release's\n"+
			"plugin manifest version also matches the tag, so a matching Claude Code plugin/skills\n"+
			"update is available too — that's a separate update through Claude Code's own plugin\n"+
			"flow, not something -Force touches.\n")
	}
	return nil
}

func fetchLatestRelease() (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "t7kb-update-check")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

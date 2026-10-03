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
		Long: "Reports whether a newer t7kb release exists, and which installs of the t7kb Claude Code\n" +
			"plugin (per scope, via `claude plugin list`) are behind it, with the commands to update\n" +
			"them. It never downloads or updates anything itself: re-run the installer with\n" +
			"-Force/--force for the binary (see the README or the setup skill).",
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
			"(a new release may bundle an updated database, uploaded separately). The installer\n"+
			"doesn't touch the Claude Code plugin; it is checked below.\n")
	}
	keep, extra := keptInstalls(t7kbInstalls())
	reportExtraScopes(out, extra)
	reportStalePlugins(out, stale(keep, latest), rel.TagName)
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

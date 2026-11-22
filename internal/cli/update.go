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
	var quiet bool
	cmd := &cobra.Command{
		Use:   "update-check",
		Short: "Check GitHub for a newer t7kb release",
		Long: "Reports whether a newer t7kb release exists, and which installs of the t7kb Claude Code\n" +
			"plugin (per scope, via `claude plugin list`) are behind it, with the commands to update\n" +
			"them. It never downloads or updates anything itself: re-run the installer with\n" +
			"-Force for the binary (see the README or the setup-or-update skill).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if quiet {
				return runQuietUpdateCheck(cmd)
			}
			return runUpdateCheck(cmd)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "print only a newer release's tag, nothing when up to date or on a dev build, and skip the plugin check (the plugin's session-start hook runs it)")
	return cmd
}

// quietTimeout bounds what a session's start waits on a slow network.
const quietTimeout = 3 * time.Second

// runQuietUpdateCheck prints the latest release's tag when this build is
// behind it, else nothing. A dev build has no release to compare, so it asks
// nothing.
func runQuietUpdateCheck(cmd *cobra.Command) error {
	if Version() == "dev" {
		return nil
	}
	rel, err := fetchLatestRelease(quietTimeout)
	if err != nil {
		return fmt.Errorf("couldn't check for updates: %w", err)
	}
	if strings.TrimPrefix(Version(), "v") != strings.TrimPrefix(rel.TagName, "v") {
		fmt.Fprintln(cmd.OutOrStdout(), rel.TagName)
	}
	return nil
}

func runUpdateCheck(cmd *cobra.Command) error {
	rel, err := fetchLatestRelease(5 * time.Second)
	if err != nil {
		return fmt.Errorf("couldn't check for updates: %w", err)
	}

	out := cmd.OutOrStdout()
	current := strings.TrimPrefix(Version(), "v")
	latest := strings.TrimPrefix(rel.TagName, "v")

	// Plain string equality, no semver: GitHub's latest release is by definition
	// newer than any other, so any difference means an update.
	switch {
	case Version() == "dev":
		fmt.Fprintf(out, "Running a local/dev build. Latest release: %s (%s)\n", rel.TagName, rel.HTMLURL)
	case current == latest:
		fmt.Fprintf(out, "Up to date (%s).\n", rel.TagName)
	default:
		fmt.Fprintf(out, "Update available: %s -> %s\n%s\n\n", Version(), rel.TagName, rel.HTMLURL)
		fmt.Fprintf(out, "Re-run the installer with -Force to fetch the new binary + database\n"+
			"(a new release may bundle an updated database, uploaded separately). The installer\n"+
			"doesn't touch the Claude Code plugin; it is checked below.\n")
	}
	keep, extra := keptInstalls(t7kbInstalls())
	reportExtraScopes(out, extra)
	reportStalePlugins(out, stale(keep, latest), rel.TagName)
	return nil
}

func fetchLatestRelease(timeout time.Duration) (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "t7kb-update-check")

	client := &http.Client{Timeout: timeout}
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

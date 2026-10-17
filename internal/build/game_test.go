package build

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The windows a French game showed on a map linked in English only, as read
// from it: the game's own window, the fatal error's dialog, and the console it
// opens beside it.
func zoneErrorWindows() []window {
	console := strings.Join([]string{
		"Command line: +set developer 2 +set logfile 2 +set fs_game zm_x +devmap zm_x",
		`Error: Waited 737 msec for missing asset "debug_sphere_exterior".`,
		"Starting up server with abort_on_error TRUE",
		`Error: couldn't exec mod.cfg`,
		`Error: Could not find material "specialty_giant_instakill_zombies".`,
		`Error: Could not find material "specialty_giant_doublepoints_zombies".`,
		`Error: Could not find material "specialty_giant_firesale_zombies".`,
		`Error: Could not find material "t7_hud_zm_powerup_giant_deathmachine".`,
		"", "",
		"ERROR: Could not find zone 'fr_zm_x'",
		"", "",
	}, "\r\n")
	return []window{
		{Class: "CoDBlackOps", Title: "Call of Duty®  - ship - Reapy"},
		{Class: dialogClass, Title: "Erreur fatale", Controls: []control{{"Button", "OK"}, {"Static", "ERROR: Could not find zone 'fr_zm_x'\n"}}},
		{Class: consoleClass, Title: "CoD Black Ops III Console", Controls: []control{{"Edit", console}}},
	}
}

func TestDialogErrorsSayTheDialogThenTheConsolesLastLines(t *testing.T) {
	got := dialogErrors(zoneErrorWindows())
	want := []string{
		`the game opened a dialog, "Erreur fatale": ERROR: Could not find zone 'fr_zm_x' [OK]`,
		`console: Error: couldn't exec mod.cfg`,
		`console: Error: Could not find material "specialty_giant_instakill_zombies".`,
		`console: Error: Could not find material "specialty_giant_doublepoints_zombies".`,
		`console: Error: Could not find material "specialty_giant_firesale_zombies".`,
		`console: Error: Could not find material "t7_hud_zm_powerup_giant_deathmachine".`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDialogErrorsReadEveryDialog(t *testing.T) {
	for name, c := range map[string]struct {
		ws   []window
		want []string
	}{
		"no dialog, the console alone": {zoneErrorWindows()[2:], nil},
		"no window":                    {nil, nil},
		"a question, its lines and buttons joined, no console": {
			[]window{{Class: dialogClass, Title: "Mode Sans échec", Controls: []control{
				{"Static", "Votre jeu ne s'est pas éteint correctement.\r\n\r\nRemarque : le mode Sans échec  modifiera vos paramètres."},
				{"Button", "&Oui"}, {"Button", "&Non"}, {"Static", ""}}}},
			[]string{`the game opened a dialog, "Mode Sans échec": Votre jeu ne s'est pas éteint correctement. Remarque : le mode Sans échec modifiera vos paramètres. [Oui] [Non]`},
		},
		"a dialog with no text": {
			[]window{{Class: dialogClass, Title: "Erreur"}},
			[]string{`the game opened a dialog, "Erreur":`},
		},
		"a console of fewer lines than the tail, one of them the dialog's": {
			[]window{
				{Class: consoleClass, Controls: []control{{"Edit", "loading\nERROR: bad\n"}}},
				{Class: dialogClass, Title: "Erreur fatale", Controls: []control{{"Static", "ERROR: bad"}}},
			},
			[]string{`the game opened a dialog, "Erreur fatale": ERROR: bad`, "console: loading"},
		},
	} {
		if got := dialogErrors(c.ws); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n%q\nwant\n%q", name, got, c.want)
		}
	}
}

func TestGameExitSaysWhenItEnded(t *testing.T) {
	old := gameStartup
	t.Cleanup(func() { gameStartup = old })
	gameStartup = 8 * time.Second
	for _, c := range []struct {
		err   error
		after time.Duration
		ok    bool
		want  string // the error, or the note of a clean quit
	}{
		{errors.New("exit status 1"), time.Second, false, "the game exited right after starting (exit status 1) — is Steam running"},
		{nil, 7999 * time.Millisecond, false, "the game exited right after starting — is Steam running"},
		{nil, 8 * time.Second, true, "launched BlackOps3.exe: it showed no dialog and quit cleanly 8s after starting"},
		{nil, 71300 * time.Millisecond, true, "launched BlackOps3.exe: it showed no dialog and quit cleanly 1m11s after starting"},
		{errors.New("exit status 0xc0000005"), 8 * time.Second, false, "the game closed 8s after starting (exit status 0xc0000005)"},
		{errors.New("exit status 3221225477"), 41600 * time.Millisecond, false, "the game closed 42s after starting (exit status 3221225477)"},
	} {
		sr := gameExit(c.err, c.after)
		got := sr.Note
		if !c.ok {
			got = strings.Join(sr.Errors, "; ")
		}
		if sr.OK != c.ok || !strings.HasPrefix(got, c.want) || (c.ok && sr.Errors != nil) || (!c.ok && sr.Note != "") {
			t.Errorf("%v after %s: %+v, want ok=%v %q", c.err, c.after, sr, c.ok, c.want)
		}
	}
}

func TestLinkLanguageIsTheAskedOneElseTheGamesElseEnglish(t *testing.T) {
	for _, c := range []struct {
		asked, localization, manifest, lang, note string
	}{
		{"german", "french" + "\r\n", "", "german", ""},
		{"", "french\r\n\r\nWIN_COULDNT_START_PROCESS\r\n", "", "french", "language french: the game's, from its localization.txt"},
		{"", "\ufeff\r\n French \r\n", "", "french", "language french: the game's, from its localization.txt"},
		{"", "WIN_COULDNT_START_PROCESS\r\n", "\t\t\"language\"\t\t\"German\"\r\n", "german", "language german: the game's, from its Steam's appmanifest_311210.acf"},
		{"", "", "\t\t\"language\"\t\t\"spanish\"\n", "spanish", "language spanish: the game's, from its Steam's appmanifest_311210.acf"},
		{"", "", "\"name\" \"x\"\n", "english", "language english: the game's install names none (localization.txt, Steam's manifest)"},
		{"", "", "", "english", "language english: the game's install names none (localization.txt, Steam's manifest)"},
	} {
		steamapps := t.TempDir()
		game := filepath.Join(steamapps, "common", "Call of Duty Black Ops III")
		if err := os.MkdirAll(game, 0o755); err != nil {
			t.Fatal(err)
		}
		if c.localization != "" {
			if err := os.WriteFile(filepath.Join(game, "localization.txt"), []byte(c.localization), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if c.manifest != "" {
			if err := os.WriteFile(filepath.Join(steamapps, "appmanifest_311210.acf"), []byte(c.manifest), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if lang, note := linkLanguage(c.asked, game); lang != c.lang || note != c.note {
			t.Errorf("asked %q, localization %q, manifest %q: %q %q", c.asked, c.localization, c.manifest, lang, note)
		}
	}
}

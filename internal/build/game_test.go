package build

import (
	"errors"
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
		want  string
	}{
		{errors.New("exit status 1"), time.Second, "the game exited right after starting (exit status 1) — is Steam running"},
		{nil, 7999 * time.Millisecond, "the game exited right after starting — is Steam running"},
		{nil, 8 * time.Second, "the game closed 8s after starting"},
		{errors.New("exit status 3221225477"), 41600 * time.Millisecond, "the game closed 42s after starting (exit status 3221225477)"},
	} {
		if got := gameExit(c.err, c.after); !strings.HasPrefix(got, c.want) {
			t.Errorf("%v after %s: %q, want %q", c.err, c.after, got, c.want)
		}
	}
}

func TestLinkLanguageIsTheAskedOneElseTheLaunchersElseEnglish(t *testing.T) {
	old := launcherLanguage
	t.Cleanup(func() { launcherLanguage = old })
	for _, c := range []struct{ asked, launcher, lang, note string }{
		{"german", "french", "german", ""},
		{"", "french", "french", "language french: the Launcher's Build Language"},
		{"", "  all \r\n", "all", "language all: the Launcher's Build Language"},
		{"", "", "english", "language english: the Launcher saves no Build Language"},
		{"", " \t", "english", "language english: the Launcher saves no Build Language"},
	} {
		launcherLanguage = func() string { return c.launcher }
		if lang, note := linkLanguage(c.asked); lang != c.lang || note != c.note {
			t.Errorf("asked %q, launcher %q: %q %q", c.asked, c.launcher, lang, note)
		}
	}
}

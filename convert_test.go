package main

import (
	"os"
	"reflect"
	"testing"
)

func TestDetectFor(t *testing.T) {
	ps := `powershell.exe -ExecutionPolicy Bypass -WindowStyle Hidden -File "D:\Vibepollo\launch.ps1" `
	cases := []struct {
		cmd  string
		want DetectHint
	}{
		{ps + `"C:\Rayman Origins\Rayman Origins.exe"`, DetectHint{Exe: `C:\Rayman Origins\Rayman Origins.exe`, InstallDir: `C:\Rayman Origins`}},
		{ps + `"steam://rungameid/8870"`, DetectHint{SteamAppid: 8870}},
		{ps + `"C:\Unravel Two\unravel.bat"`, DetectHint{InstallDir: `C:\Unravel Two`}},
		{`"C:\game.exe"`, DetectHint{Exe: `C:\game.exe`}},
		{`cmd /C rundll32.exe powrprof.dll,SetSuspendState 0,1,0`, DetectHint{}},
	}
	for _, c := range cases {
		if got, _ := detectFor(c.cmd); got != c.want {
			t.Errorf("detectFor(%q) = %+v, want %+v", c.cmd, got, c.want)
		}
	}
}

func TestParseSelection(t *testing.T) {
	allowed := []int{1, 2, 4, 5, 6}
	got, err := parseSelection("1, 4-6", allowed)
	if err != nil || !reflect.DeepEqual(got, []int{1, 4, 5, 6}) {
		t.Fatalf("got %v %v", got, err)
	}
	if got, _ := parseSelection("", allowed); !reflect.DeepEqual(got, allowed) {
		t.Fatalf("empty = %v", got)
	}
	if _, err := parseSelection("3", allowed); err == nil {
		t.Fatal("3 is not importable")
	}
	if _, err := parseSelection("5-4", allowed); err == nil {
		t.Fatal("reversed range")
	}
}

func TestSampleAppsJSON(t *testing.T) {
	f, err := loadSunshine("testdata/apps.json")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Plan{}
	for _, a := range f.Apps {
		by[cleanTitle(a.Name)] = planApp(a)
	}
	for _, skip := range []string{"Desktop", "Steam Big Picture"} {
		if by[skip].Skip == "" {
			t.Errorf("%s should be skipped", skip)
		}
	}

	// A game launched by its .exe: that exe and its folder.
	h := by["Hollow Knight"]
	if d := h.Input.Detect; h.Skip != "" || d == nil || d.Exe != `C:\Games\Hollow Knight\hollow_knight.exe` || d.InstallDir != `C:\Games\Hollow Knight` {
		t.Errorf("Hollow Knight: %+v", h)
	}
	if h.Input.Launch.Value != `cd /d "C:\Games\Hollow Knight" && "C:\Games\Hollow Knight\hollow_knight.exe"` || h.IgdbID != "14593" {
		t.Errorf("Hollow Knight launch/cover: %q %q", h.Input.Launch.Value, h.IgdbID)
	}

	// A Steam game: its appid; the placeholder cmd is dropped.
	c := by["Celeste (Steam)"]
	if d := c.Input.Detect; d == nil || d.SteamAppid != 504230 || c.Input.Launch.Value != "steam://rungameid/504230" {
		t.Errorf("Celeste: %+v", c)
	}

	// Through a wrapper: PowerShell is skipped, the game's exe is watched, the command is kept.
	r := by["Rayman Origins"]
	if d := r.Input.Detect; d == nil || d.Exe != `D:\Juegos\Rayman Origins\Rayman Origins.exe` {
		t.Errorf("Rayman detect: %+v", r.Input.Detect)
	}
	if r.Input.Launch.Value != f.Apps[4].Cmd || r.IgdbID != "981" || len(r.Input.Prep) != 1 || r.Input.Prep[0].Undo == nil {
		t.Errorf("Rayman: %+v", r.Input)
	}

	// No exe in the command: imported, with a warning, and the local cover flagged.
	m := by["Minecraft (Microsoft Store)"]
	if m.Skip != "" || m.Input.Detect != nil || len(m.Warnings) != 2 {
		t.Errorf("Minecraft: %+v", m)
	}

	// Suspend: nothing to watch, and no warning for it.
	if s := by["Suspender PC"]; s.Skip != "" || len(s.Warnings) != 0 {
		t.Errorf("Suspender PC: %+v", s)
	}
}

func TestSteamInLibrary(t *testing.T) {
	lib := []LibraryEntry{{ID: "steam:504230", Title: "Celeste"}, {ID: "custom:a1", Title: "Hollow Knight"}}
	if got := steamInLibrary(lib, 504230); got != "steam:504230" {
		t.Errorf("got %q", got)
	}
	if steamInLibrary(lib, 8870) != "" || steamInLibrary(lib, 0) != "" {
		t.Error("only the same appid counts")
	}
}

func TestDefaultStateFileMigrates(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/punkavibe-state.json", []byte(`{"entries":{}}`), 0o644)
	p := defaultStateFile(dir)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("old state not moved to %s", p)
	}
}

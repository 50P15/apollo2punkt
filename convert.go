package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SunshineFile is apps.json as Sunshine, Apollo and Vibepollo write it.
type SunshineFile struct {
	Env  map[string]string `json:"env"`
	Apps []SunshineApp     `json:"apps"`
}

// SunshineApp holds the fields the import reads. Vibepollo-only keys are kept raw so the
// report can name the ones Punktfunk has no equivalent for.
type SunshineApp struct {
	Name       string         `json:"name"`
	UUID       string         `json:"uuid"`
	Cmd        string         `json:"cmd"`
	Detached   []string       `json:"detached"`
	PrepCmd    []SunshinePrep `json:"prep-cmd"`
	WorkingDir string         `json:"working-dir"`
	ImagePath  string         `json:"image-path"`
	Elevated   jsonBool       `json:"elevated"`
	Raw        map[string]any `json:"-"`
}

type SunshinePrep struct {
	Do       string   `json:"do"`
	Undo     string   `json:"undo"`
	Elevated jsonBool `json:"elevated"`
}

// jsonBool accepts true/false and the "true"/"false" strings older Sunshine builds wrote.
type jsonBool bool

func (b *jsonBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	*b = jsonBool(strings.EqualFold(s, "true"))
	return nil
}

func loadSunshine(path string) (*SunshineFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = []byte(strings.TrimPrefix(string(raw), "\ufeff"))
	var f SunshineFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s no es un apps.json válido: %w", path, err)
	}
	var loose struct {
		Apps []map[string]any `json:"apps"`
	}
	if err := json.Unmarshal(raw, &loose); err == nil && len(loose.Apps) == len(f.Apps) {
		for i := range f.Apps {
			f.Apps[i].Raw = loose.Apps[i]
		}
	}
	return &f, nil
}

// --- Punktfunk wire types (POST /api/v1/library/custom) ---

type LaunchSpec struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type PrepCmd struct {
	Do       string  `json:"do"`
	Undo     *string `json:"undo,omitempty"`
	Elevated bool    `json:"elevated,omitempty"`
}

type DetectHint struct {
	InstallDir string `json:"install_dir,omitempty"`
	Exe        string `json:"exe,omitempty"`
	SteamAppid uint32 `json:"steam_appid,omitempty"`
}

func (d DetectHint) empty() bool { return d.InstallDir == "" && d.Exe == "" && d.SteamAppid == 0 }

type Artwork struct {
	Portrait string `json:"portrait,omitempty"`
	Hero     string `json:"hero,omitempty"`
}

type CustomInput struct {
	Title  string      `json:"title"`
	Art    Artwork     `json:"art"`
	Launch *LaunchSpec `json:"launch,omitempty"`
	Prep   []PrepCmd   `json:"prep"`
	Detect *DetectHint `json:"detect,omitempty"`
}

// Plan is what the import does with one app.
type Plan struct {
	App      SunshineApp
	Skip     string // reason; empty means import
	Input    CustomInput
	IgdbID   string
	Warnings []string
}

var (
	steamRunRe = regexp.MustCompile(`(?i)steam://(?:rungameid|run)/(\d{1,10})`)
	igdbRe     = regexp.MustCompile(`(?i)(?:^|[\\/])igdb_(\d{1,10})\.(?:png|jpe?g|webp)$`)
	wsRe       = regexp.MustCompile(`\s+`)
	// Programs that only hand the game on: never the game itself.
	wrappers = map[string]bool{
		"powershell.exe": true, "pwsh.exe": true, "cmd.exe": true, "psexec.exe": true,
		"psexec64.exe": true, "steam.exe": true, "explorer.exe": true, "start.exe": true,
		"conhost.exe": true, "wscript.exe": true, "cscript.exe": true,
	}
	trivialCmd = regexp.MustCompile(`(?i)^\s*(timeout(\.exe)?\s+/t\s+0|rem\b.*|exit(\s+0)?)?\s*$`)
)

// splitCommandLine splits like CommandLineToArgvW for the cases apps.json holds: double quotes
// group, backslashes are literal (Windows paths).
func splitCommandLine(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote, have := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			have = true
		case (r == ' ' || r == '\t') && !inQuote:
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if have {
		out = append(out, cur.String())
	}
	return out
}

func isAbsWindows(p string) bool {
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z'))
}

// winDir is filepath.Dir for a Windows path on any OS.
func winDir(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	i := strings.LastIndex(p, `\`)
	if i < 0 {
		return ""
	}
	if i == 2 { // "C:\x" → "C:\"
		return p[:3]
	}
	return p[:i]
}

func winBase(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	return p[strings.LastIndex(p, `\`)+1:]
}

// detectFor finds what the game is in a command: a Steam appid, or the last absolute path to a
// program that is not a wrapper (PowerShell, PsExec, cmd…). A script names only its folder.
func detectFor(cmd string) (DetectHint, string) {
	if m := steamRunRe.FindStringSubmatch(cmd); m != nil {
		var id uint32
		fmt.Sscanf(m[1], "%d", &id)
		return DetectHint{SteamAppid: id}, "Steam " + m[1]
	}
	var target string
	for _, tok := range splitCommandLine(cmd) {
		tok = strings.TrimSpace(tok)
		if !isAbsWindows(tok) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(tok))
		switch ext {
		case ".exe", ".bat", ".cmd", ".lnk":
		default:
			continue
		}
		if wrappers[strings.ToLower(winBase(tok))] {
			continue
		}
		target = tok
	}
	if target == "" {
		return DetectHint{}, ""
	}
	target = strings.ReplaceAll(target, "/", `\`)
	dir := winDir(target)
	rootDir := len(dir) <= 3
	var h DetectHint
	if strings.EqualFold(filepath.Ext(target), ".exe") {
		h.Exe = target
	}
	if !rootDir {
		h.InstallDir = dir
	}
	if h.empty() {
		return h, ""
	}
	if h.Exe != "" {
		return h, "exe " + h.Exe
	}
	return h, "carpeta " + h.InstallDir
}

// igdbIDFor is the IGDB id in a Vibepollo/Apollo cover file name (covers/igdb_981.png).
func igdbIDFor(imagePath string) string {
	if m := igdbRe.FindStringSubmatch(strings.TrimSpace(imagePath)); m != nil {
		return m[1]
	}
	return ""
}

// Keys with no Punktfunk equivalent, worth a warning only when set to something other than
// Vibepollo's default.
var vibepolloOnly = map[string]func(v any) bool{
	"lossless-scaling-enabled":  truthy,
	"lossless-scaling-framegen": truthy,
	"frame-generation-mode":     func(v any) bool { s, _ := v.(string); return s != "" && s != "off" },
	"gen1-framegen-fix":         truthy,
	"gen2-framegen-fix":         truthy,
	"scale-factor":              func(v any) bool { f, ok := v.(float64); return ok && f != 100 },
	"terminate-on-pause":        truthy,
	"use-app-identity":          truthy,
	"per-client-app-identity":   truthy,
	"dualsense-haptics":         truthy,
	"gamepad":                   func(v any) bool { s, _ := v.(string); return s != "" },
	"config-overrides": func(v any) bool {
		m, ok := v.(map[string]any)
		return ok && len(m) > 0
	},
	"state-cmd": func(v any) bool { a, ok := v.([]any); return ok && len(a) > 0 },
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return false
}

func cleanTitle(s string) string { return strings.TrimSpace(wsRe.ReplaceAllString(s, " ")) }

// planApp turns one apps.json entry into a library entry, or says why it is left out.
func planApp(a SunshineApp) Plan {
	p := Plan{App: a}
	cmd := strings.TrimSpace(a.Cmd)
	var detached []string
	for _, d := range a.Detached {
		if d = strings.TrimSpace(d); d != "" {
			detached = append(detached, d)
		}
	}
	lower := strings.ToLower(cmd + " " + strings.Join(detached, " "))
	switch {
	case cleanTitle(a.Name) == "":
		p.Skip = "sin nombre"
		return p
	case strings.Contains(lower, "steam://open/bigpicture"):
		p.Skip = "Steam Big Picture: instala el plugin oficial de Steam, que añade su propio acceso"
		return p
	case cmd == "" && len(detached) == 0:
		p.Skip = "no lanza nada (Desktop / Remote Input / Remote Monitor: Punktfunk ya trae Desktop)"
		return p
	}

	// Sunshine runs every detached command, then cmd. A placeholder cmd ("timeout /t 0") with
	// detached commands is the usual "run and return" shape.
	var parts []string
	parts = append(parts, detached...)
	if !trivialCmd.MatchString(cmd) {
		parts = append(parts, cmd)
	}
	if len(parts) == 0 {
		p.Skip = "el comando no hace nada"
		return p
	}
	value := strings.Join(parts, " & ")
	if len(parts) > 1 {
		p.Warnings = append(p.Warnings, "varios comandos unidos con '&': revisa que el orden sea el esperado")
	}
	if wd := strings.TrimSpace(a.WorkingDir); wd != "" {
		value = fmt.Sprintf(`cd /d "%s" && %s`, strings.Trim(wd, `"`), value)
	}
	if a.Elevated {
		p.Warnings = append(p.Warnings, "estaba marcado como 'elevated': Punktfunk lo lanza sin elevar")
	}

	p.Input = CustomInput{
		Title:  cleanTitle(a.Name),
		Launch: &LaunchSpec{Kind: "command", Value: value},
		Prep:   []PrepCmd{},
	}
	for _, pc := range a.PrepCmd {
		do, undo := strings.TrimSpace(pc.Do), strings.TrimSpace(pc.Undo)
		if do == "" && undo == "" {
			continue
		}
		if do == "" {
			do = "rem"
		}
		c := PrepCmd{Do: do, Elevated: bool(pc.Elevated)}
		if undo != "" {
			c.Undo = &undo
		}
		p.Input.Prep = append(p.Input.Prep, c)
	}

	if h, _ := detectFor(value); !h.empty() {
		p.Input.Detect = &h
	} else if !strings.Contains(lower, "setsuspendstate") {
		p.Warnings = append(p.Warnings, "no se encontró el juego en el comando: Punktfunk no sabrá cuándo se cierra")
	}

	p.IgdbID = igdbIDFor(a.ImagePath)
	if p.IgdbID == "" && strings.TrimSpace(a.ImagePath) != "" {
		p.Warnings = append(p.Warnings, "carátula local sin id de IGDB ("+a.ImagePath+"): búscala luego en Punktfunk")
	}

	for key, set := range vibepolloOnly {
		if v, ok := a.Raw[key]; ok && set(v) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("'%s' no tiene equivalente en Punktfunk (valor: %v)", key, v))
		}
	}
	return p
}

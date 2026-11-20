// apollo2punkt imports the games from a Vibepollo / Apollo / Sunshine apps.json into the
// Punktfunk library on the same PC, keeping each game's launch command as it is.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var version = "1.0.0"

var stdin = bufio.NewReader(os.Stdin)

func ask(prompt string) string {
	fmt.Print(prompt)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

func yes(prompt string) bool {
	a := strings.ToLower(ask(prompt + " [s/N]: "))
	return a == "s" || a == "si" || a == "sí" || a == "y" || a == "yes"
}

func exeDir() string {
	if p, err := os.Executable(); err == nil {
		return filepath.Dir(p)
	}
	return "."
}

// candidates are the apps.json files that exist on this PC, most likely first.
func candidates() []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[strings.ToLower(p)] {
			return
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			seen[strings.ToLower(p)] = true
			out = append(out, p)
		}
	}
	add(filepath.Join(exeDir(), "apps.json"))
	names := []string{"Vibepollo", "Apollo", "Sunshine"}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			for _, n := range names {
				add(filepath.Join(pf, n, "config", "apps.json"))
			}
		}
	}
	for d := 'C'; d <= 'Z'; d++ {
		root := string(d) + `:\`
		if _, err := os.Stat(root); err != nil {
			continue
		}
		for _, n := range names {
			add(filepath.Join(root, n, "config", "apps.json"))
		}
	}
	return out
}

// parseSelection reads "1,3-5" against the allowed numbers; empty means all of them.
func parseSelection(s string, allowed []int) ([]int, error) {
	ok := map[int]bool{}
	for _, n := range allowed {
		ok[n] = true
	}
	if strings.TrimSpace(s) == "" || strings.EqualFold(strings.TrimSpace(s), "todos") {
		return allowed, nil
	}
	picked := map[int]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			return nil, fmt.Errorf("no entiendo %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil || b < a {
				return nil, fmt.Errorf("rango no válido %q", part)
			}
		}
		for n := a; n <= b; n++ {
			if !ok[n] {
				return nil, fmt.Errorf("el %d no se puede importar", n)
			}
			picked[n] = true
		}
	}
	var out []int
	for n := range picked {
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func trunc(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func printPlans(plans []Plan) {
	fmt.Println()
	for i, p := range plans {
		n := i + 1
		if p.Skip != "" {
			fmt.Printf(" --  %-36s se omite: %s\n", trunc(cleanTitle(p.App.Name), 36), p.Skip)
			continue
		}
		det := "sin detección"
		if d := p.Input.Detect; d != nil {
			switch {
			case d.SteamAppid != 0:
				det = fmt.Sprintf("Steam %d", d.SteamAppid)
			case d.Exe != "":
				det = d.Exe
			default:
				det = "carpeta " + d.InstallDir
			}
		}
		cover := "sin carátula"
		if p.IgdbID != "" {
			cover = "IGDB " + p.IgdbID
		}
		fmt.Printf("%3d  %-36s %s | %s\n", n, trunc(p.Input.Title, 36), cover, trunc(det, 60))
		for _, w := range p.Warnings {
			fmt.Printf("       ! %s\n", w)
		}
	}
	fmt.Println()
}

type options struct {
	apps, export, url, tokenFile, stateFile  string
	yes, update, undo, noArt, noPause, steam bool
}

func main() {
	var o options
	flag.StringVar(&o.apps, "apps", "", "ruta del apps.json (por defecto se busca solo)")
	flag.BoolVar(&o.yes, "yes", false, "importar todo sin preguntar")
	flag.BoolVar(&o.update, "update", false, "sobrescribir los juegos ya importados")
	flag.BoolVar(&o.undo, "undo", false, "borrar de Punktfunk los juegos que importó este programa")
	flag.StringVar(&o.export, "export", "", "no tocar Punktfunk: guardar en este archivo lo que se enviaría")
	flag.BoolVar(&o.noArt, "no-art", false, "no buscar carátulas en GameDB")
	flag.BoolVar(&o.steam, "steam", false, "importar también los juegos de Steam que ya añade el plugin de Steam")
	flag.StringVar(&o.url, "url", "", "URL de la API de gestión (por defecto la de mgmt-endpoint)")
	flag.StringVar(&o.tokenFile, "token-file", "", "archivo mgmt-token (por defecto en %ProgramData%\\punktfunk)")
	flag.StringVar(&o.stateFile, "state", "", "archivo de estado (por defecto apollo2punkt-state.json junto al .exe)")
	flag.BoolVar(&o.noPause, "no-pause", false, "no esperar a Enter al terminar")
	flag.Parse()
	if o.stateFile == "" {
		o.stateFile = defaultStateFile(exeDir())
	}

	fmt.Printf("apollo2punkt %s — importa juegos de Vibepollo/Apollo/Sunshine a Punktfunk\n", version)
	err := run(o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
	}
	if !o.noPause && !o.yes {
		ask("\nPulsa Enter para salir...")
	}
	if err != nil {
		os.Exit(1)
	}
}

func run(o options) error {
	if o.undo {
		return undo(o)
	}

	path := o.apps
	if path == "" {
		found := candidates()
		switch {
		case len(found) == 0:
			p := ask("No encuentro apps.json. Escribe su ruta (o arrástralo aquí): ")
			path = strings.Trim(p, `"`)
		case len(found) == 1 || o.yes:
			path = found[0]
		default:
			for i, f := range found {
				fmt.Printf("%d) %s\n", i+1, f)
			}
			n, err := strconv.Atoi(ask("¿Cuál uso? [1]: ") + "")
			if err != nil || n < 1 || n > len(found) {
				n = 1
			}
			path = found[n-1]
		}
	}
	if path == "" {
		return errors.New("no se indicó ningún apps.json")
	}
	fmt.Println("Leyendo", path)
	file, err := loadSunshine(path)
	if err != nil {
		return err
	}

	plans := make([]Plan, len(file.Apps))
	var importable []int
	for i, a := range file.Apps {
		plans[i] = planApp(a)
		if plans[i].Skip == "" {
			importable = append(importable, i+1)
		}
	}
	printPlans(plans)
	if len(importable) == 0 {
		return errors.New("no hay nada que importar")
	}

	selected := importable
	if !o.yes {
		for {
			s := ask("¿Qué juegos importo? (ej. 1,3-5; Enter = todos; q = salir): ")
			if strings.EqualFold(s, "q") {
				return errors.New("cancelado")
			}
			if selected, err = parseSelection(s, importable); err == nil {
				break
			}
			fmt.Println(" ", err)
		}
	}

	if !o.noArt {
		fmt.Println("Buscando carátulas en GameDB…")
		web := newWebClient()
		for _, n := range selected {
			p := &plans[n-1]
			if p.IgdbID == "" {
				continue
			}
			art, name, err := artFor(web, p.IgdbID)
			if err != nil {
				p.Warnings = append(p.Warnings, "carátula: "+err.Error())
				fmt.Printf("  %-36s %v\n", trunc(p.Input.Title, 36), err)
				continue
			}
			p.Input.Art = art
			fmt.Printf("  %-36s %s\n", trunc(p.Input.Title, 36), name)
		}
	}

	if o.export != "" {
		var out []CustomInput
		for _, n := range selected {
			out = append(out, plans[n-1].Input)
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		if err := os.WriteFile(o.export, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("\nGuardado en %s (%d juegos). No se ha tocado Punktfunk.\n", o.export, len(out))
		return nil
	}

	api, err := newAPI(configDir(), o.url, o.tokenFile)
	if err != nil {
		return err
	}
	if !api.Pinned {
		fmt.Println("Aviso: no encontré el certificado del host; la conexión no verifica su identidad.")
	}
	lib, err := api.Library()
	if err != nil {
		return fmt.Errorf("no puedo hablar con Punktfunk en %s: %w", api.base, err)
	}
	state, err := loadState(o.stateFile)
	if err != nil {
		return err
	}

	if !o.yes && !yes(fmt.Sprintf("\nSe van a enviar %d juegos a Punktfunk (%s). ¿Continuar?", len(selected), api.base)) {
		return errors.New("cancelado")
	}

	var created, updated, skipped, failed int
	for _, n := range selected {
		p := plans[n-1]
		key := stateKey(p.App)
		title := p.Input.Title
		id := ""
		if st, ok := state.Entries[key]; ok {
			exists, err := api.Exists(st.ID)
			if err != nil {
				fmt.Printf("  ERROR    %s: %v\n", title, err)
				failed++
				continue
			}
			if exists {
				id = st.ID
			}
		}
		if id == "" && !o.steam && p.Input.Detect != nil {
			if dup := steamInLibrary(lib, p.Input.Detect.SteamAppid); dup != "" {
				fmt.Printf("  omitido  %s (ya lo añade el plugin de Steam como %s; usa -steam para importarlo igualmente)\n", title, dup)
				skipped++
				continue
			}
		}
		if id == "" {
			for _, e := range lib {
				if strings.EqualFold(cleanTitle(e.Title), title) {
					if strings.HasPrefix(e.ID, "custom:") {
						id = strings.TrimPrefix(e.ID, "custom:")
					} else {
						fmt.Printf("  aviso    %s ya está en la biblioteca como %s; se añade igualmente\n", title, e.ID)
					}
				}
			}
		}
		switch {
		case id != "" && !o.update:
			fmt.Printf("  omitido  %s (ya existe; usa -update para sobrescribirlo)\n", title)
			skipped++
			continue
		case id != "":
			if err := api.Update(id, p.Input); err != nil {
				fmt.Printf("  ERROR    %s: %v\n", title, err)
				failed++
				continue
			}
			fmt.Printf("  actualizado %s\n", title)
			updated++
		default:
			newID, err := api.Create(p.Input)
			if err != nil {
				fmt.Printf("  ERROR    %s: %v\n", title, err)
				failed++
				continue
			}
			id = newID
			fmt.Printf("  añadido  %s\n", title)
			created++
		}
		state.Entries[key] = StateEntry{ID: id, Title: title}
		if err := state.save(o.stateFile); err != nil {
			fmt.Printf("  aviso: no pude guardar %s: %v\n", o.stateFile, err)
		}
	}
	fmt.Printf("\nHecho: %d añadidos, %d actualizados, %d omitidos, %d con error.\n", created, updated, skipped, failed)
	if created+updated > 0 {
		fmt.Println("Ábrelos en la consola web de Punktfunk (Biblioteca) y prueba a lanzar uno.")
	}
	if failed > 0 {
		return fmt.Errorf("%d juegos fallaron", failed)
	}
	return nil
}

// steamInLibrary is the library id of a Steam game another source (the Steam plugin) already
// lists, or "" when there is none.
func steamInLibrary(lib []LibraryEntry, appid uint32) string {
	if appid == 0 {
		return ""
	}
	want := fmt.Sprintf("steam:%d", appid)
	for _, e := range lib {
		if strings.EqualFold(e.ID, want) {
			return e.ID
		}
	}
	return ""
}

// defaultStateFile sits beside the .exe. A state file from punkavibe, this program's old name,
// is renamed so -update and -undo still know what it imported.
func defaultStateFile(dir string) string {
	p := filepath.Join(dir, "apollo2punkt-state.json")
	old := filepath.Join(dir, "punkavibe-state.json")
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(old); err == nil {
			_ = os.Rename(old, p)
		}
	}
	return p
}

func undo(o options) error {
	state, err := loadState(o.stateFile)
	if err != nil {
		return err
	}
	if len(state.Entries) == 0 {
		return fmt.Errorf("no hay nada que deshacer (%s vacío o inexistente)", o.stateFile)
	}
	keys := make([]string, 0, len(state.Entries))
	for k := range state.Entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return state.Entries[keys[i]].Title < state.Entries[keys[j]].Title })
	fmt.Println("\nSe borrarán de Punktfunk:")
	for _, k := range keys {
		fmt.Println("  -", state.Entries[k].Title)
	}
	if !o.yes && !yes("¿Continuar?") {
		return errors.New("cancelado")
	}
	api, err := newAPI(configDir(), o.url, o.tokenFile)
	if err != nil {
		return err
	}
	failed := 0
	for _, k := range keys {
		e := state.Entries[k]
		if err := api.Delete(e.ID); err != nil {
			fmt.Printf("  ERROR  %s: %v\n", e.Title, err)
			failed++
			continue
		}
		delete(state.Entries, k)
		fmt.Printf("  borrado  %s\n", e.Title)
	}
	if err := state.save(o.stateFile); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d no se pudieron borrar", failed)
	}
	return nil
}

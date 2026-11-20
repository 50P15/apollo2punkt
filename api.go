package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultURL = "https://127.0.0.1:47990"

// configDir is where the Windows host keeps mgmt-token, mgmt-endpoint and its certificate.
func configDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "punktfunk")
}

// readKV reads KEY=value from an env-style file.
func readKV(path, key string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`), nil
		}
	}
	return "", fmt.Errorf("%s no contiene %s", path, key)
}

type API struct {
	base   string
	token  string
	client *http.Client
	Pinned bool
}

func newAPI(dir, urlFlag, tokenFile string) (*API, error) {
	if tokenFile == "" {
		tokenFile = filepath.Join(dir, "mgmt-token")
	}
	token, err := readKV(tokenFile, "PUNKTFUNK_MGMT_TOKEN")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("no se puede leer %s: ejecuta el programa como administrador", tokenFile)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no existe %s: ¿está Punktfunk instalado y arrancado en este PC?", tokenFile)
		}
		return nil, err
	}
	base := urlFlag
	if base == "" {
		base, _ = readKV(filepath.Join(dir, "mgmt-endpoint"), "PUNKTFUNK_MGMT_URL")
	}
	if base == "" {
		base = defaultURL
	}
	base = strings.TrimRight(base, "/")
	if u, err := url.Parse(base); err != nil || u.Host == "" {
		return nil, fmt.Errorf("URL de gestión no válida: %q", base)
	}

	a := &API{base: base, token: token}
	// The host certificate is self-signed: pin the one on disk when there is one.
	var pinned [][]byte
	for _, name := range []string{"native-cert.pem", "cert.pem"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for rest := raw; ; {
			var b *pem.Block
			b, rest = pem.Decode(rest)
			if b == nil {
				break
			}
			if b.Type == "CERTIFICATE" {
				pinned = append(pinned, b.Bytes)
			}
		}
	}
	tlsCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // verified below by pin
	if len(pinned) > 0 {
		a.Pinned = true
		tlsCfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("el host no presentó certificado")
			}
			for _, p := range pinned {
				if bytes.Equal(p, raw[0]) {
					return nil
				}
			}
			return errors.New("el certificado del host no coincide con el de " + dir)
		}
	}
	a.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: nil},
	}
	return a, nil
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	msg := strings.TrimSpace(e.Body)
	var j struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(msg), &j) == nil {
		if j.Error != "" {
			msg = j.Error
		} else if j.Message != "" {
			msg = j.Message
		}
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, msg)
}

func (a *API) do(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &apiError{Status: res.StatusCode, Body: string(data)}
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func isStatus(err error, code int) bool {
	var e *apiError
	return errors.As(err, &e) && e.Status == code
}

type LibraryEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (a *API) Library() ([]LibraryEntry, error) {
	var out []LibraryEntry
	return out, a.do(http.MethodGet, "/api/v1/library", nil, &out)
}

func (a *API) Create(in CustomInput) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := a.do(http.MethodPost, "/api/v1/library/custom", in, &out); err != nil {
		return "", err
	}
	return strings.TrimPrefix(out.ID, "custom:"), nil
}

func (a *API) Update(id string, in CustomInput) error {
	return a.do(http.MethodPut, "/api/v1/library/custom/"+url.PathEscape(id), in, nil)
}

// Exists reports whether a custom entry is still there.
func (a *API) Exists(id string) (bool, error) {
	err := a.do(http.MethodGet, "/api/v1/library/custom/"+url.PathEscape(id), nil, nil)
	if isStatus(err, http.StatusNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (a *API) Delete(id string) error {
	err := a.do(http.MethodDelete, "/api/v1/library/custom/"+url.PathEscape(id), nil, nil)
	if isStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
}

// State maps a Sunshine app uuid (or its title) to the custom entry the import made, so a
// second run updates or skips instead of duplicating, and -undo knows what to delete.
type State struct {
	Entries map[string]StateEntry `json:"entries"`
}

type StateEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func stateKey(a SunshineApp) string {
	if u := strings.TrimSpace(a.UUID); u != "" {
		return strings.ToUpper(u)
	}
	return "title:" + strings.ToLower(cleanTitle(a.Name))
}

func loadState(path string) (*State, error) {
	s := &State{Entries: map[string]StateEntry{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("%s está dañado: %w", path, err)
	}
	if s.Entries == nil {
		s.Entries = map[string]StateEntry{}
	}
	return s, nil
}

func (s *State) save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

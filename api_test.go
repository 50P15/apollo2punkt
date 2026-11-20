package main

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAPIPinnedRoundTrip(t *testing.T) {
	var got CustomInput
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/library":
			w.Write([]byte(`[{"id":"custom:x1","title":"Rayman Origins"}]`))
		case "POST /api/v1/library/custom":
			json.NewDecoder(r.Body).Decode(&got)
			w.Write([]byte(`{"id":"n1","title":"A"}`))
		case "GET /api/v1/library/custom/gone":
			w.WriteHeader(http.StatusNotFound)
		case "PUT /api/v1/library/custom/n1":
			w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte(`{"error":"nope"}`))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	os.WriteFile(filepath.Join(dir, "native-cert.pem"), cert, 0o600)
	os.WriteFile(filepath.Join(dir, "mgmt-token"), []byte("\ufeffPUNKTFUNK_MGMT_TOKEN=abc\r\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "mgmt-endpoint"), []byte("PUNKTFUNK_MGMT_URL="+srv.URL+"\n"), 0o600)

	a, err := newAPI(dir, "", "")
	if err != nil || !a.Pinned {
		t.Fatalf("newAPI: %v pinned=%v", err, a != nil && a.Pinned)
	}
	lib, err := a.Library()
	if err != nil || len(lib) != 1 || lib[0].ID != "custom:x1" {
		t.Fatalf("library %v %v", lib, err)
	}
	id, err := a.Create(CustomInput{Title: "A", Launch: &LaunchSpec{Kind: "command", Value: "x"}, Prep: []PrepCmd{}})
	if err != nil || id != "n1" || got.Launch.Value != "x" {
		t.Fatalf("create %q %v %+v", id, err, got)
	}
	if err := a.Update("n1", got); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.Exists("gone"); ok || err != nil {
		t.Fatalf("exists %v %v", ok, err)
	}
	if err := a.Delete("other"); err == nil || err.Error() != "HTTP 418: nope" {
		t.Fatalf("delete err %v", err)
	}

	// A different certificate on disk must be refused.
	other := httptest.NewTLSServer(http.NotFoundHandler())
	defer other.Close()
	b, _ := newAPI(dir, other.URL, "")
	if _, err := b.Library(); err == nil {
		t.Fatal("pin not enforced")
	}
}

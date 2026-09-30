package navidrome

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestPlaylistPaths(t *testing.T) {
	const total = pageSize + 2
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		var creds map[string]string
		json.NewDecoder(r.Body).Decode(&creds) //nolint:errcheck
		if creds["username"] != "me" || creds["password"] != "secret" {
			http.Error(w, "bad login", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"token":"tok"}`)
	})
	mux.HandleFunc("GET /api/playlist/pl1/tracks", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-nd-authorization") != "Bearer tok" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		start, _ := strconv.Atoi(r.URL.Query().Get("_start"))
		end, _ := strconv.Atoi(r.URL.Query().Get("_end"))
		var tracks []map[string]string
		for i := start; i < min(end, total); i++ {
			tracks = append(tracks, map[string]string{"path": fmt.Sprintf("Artist/Album/%03d.flac", i)})
		}
		json.NewEncoder(w).Encode(tracks) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := Login(t.Context(), srv.URL, "me", "wrong"); err == nil {
		t.Fatal("expected error for wrong password")
	}
	c, err := Login(t.Context(), srv.URL, "me", "secret")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := c.PlaylistPaths(t.Context(), "pl1")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != total || paths[total-1] != fmt.Sprintf("Artist/Album/%03d.flac", total-1) {
		t.Errorf("got %d paths, last %q", len(paths), paths[len(paths)-1])
	}
}

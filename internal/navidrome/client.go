// Package navidrome reads playlists from a Navidrome server through its
// native API, the one the web UI and Feishin use. The Subsonic API only
// reports made-up paths, so it cannot be matched to files.
package navidrome

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// pageSize is how many playlist tracks are fetched per request.
const pageSize = 500

// Client talks to one Navidrome server as one user.
type Client struct {
	baseURL string
	http    *http.Client
	token   string
}

// Login signs in and returns a client that sends the session token.
func Login(ctx context.Context, baseURL, username, password string) (*Client, error) {
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c := &Client{baseURL: baseURL, http: http.DefaultClient}
	var resp struct {
		Token string `json:"token"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("navidrome login: %w", err)
	}
	if resp.Token == "" {
		return nil, fmt.Errorf("navidrome login: no token in response")
	}
	c.token = resp.Token
	return c, nil
}

// PlaylistPaths returns the path of every track in the playlist, relative
// to the Navidrome library folder, in playlist order.
func (c *Client) PlaylistPaths(ctx context.Context, playlistID string) ([]string, error) {
	var paths []string
	for start := 0; ; start += pageSize {
		q := url.Values{"_start": {strconv.Itoa(start)}, "_end": {strconv.Itoa(start + pageSize)}}
		u := c.baseURL + "/api/playlist/" + url.PathEscape(playlistID) + "/tracks?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-nd-authorization", "Bearer "+c.token)
		var tracks []struct {
			Path string `json:"path"`
		}
		if err := c.do(req, &tracks); err != nil {
			return nil, fmt.Errorf("playlist %s: %w", playlistID, err)
		}
		for _, t := range tracks {
			paths = append(paths, t.Path)
		}
		if len(tracks) < pageSize {
			return paths, nil
		}
	}
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s", req.Method, req.URL.Path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

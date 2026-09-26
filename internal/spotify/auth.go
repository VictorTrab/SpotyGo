package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	DefaultClientID = "d420a117a32841c2b3474932e49fb54b"
	redirectURI    = "http://127.0.0.1:8989/login"
	keyringService = "SpotyGo Spotify"
	scopes         = "user-read-playback-state user-modify-playback-state user-read-currently-playing streaming app-remote-control playlist-read-private playlist-read-collaborative user-library-read user-read-recently-played user-top-read"
)

type savedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       string    `json:"scopes"`
}

// Auth keeps the current Spotify grant in the operating system's credential store.
type Auth struct {
	clientID string
	http     *http.Client
	mu       sync.Mutex
	loaded   bool
	token    savedToken
}

func NewAuth(clientID string) *Auth {
	if clientID == "" {
		clientID = DefaultClientID
	}
	return &Auth{clientID: clientID, http: &http.Client{Timeout: 12 * time.Second}}
}

// ClientID returns the Spotify application client ID in use.
func (a *Auth) ClientID() string { return a.clientID }

// Scopes returns the OAuth scopes granted for the stored token.
func (a *Auth) Scopes() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token.Scopes
}

func (a *Auth) AccessToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.loaded {
		stored, err := keyring.Get(keyringService, a.clientID)
		if err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("leer credenciales de Spotify: %w", err)
		}
		if err == nil {
			if err := json.Unmarshal([]byte(stored), &a.token); err != nil {
				return "", fmt.Errorf("credenciales guardadas inválidas: %w", err)
			}
		} else {
			if imported, ok := tryImportSpotifyPlayerToken(a.clientID); ok {
				a.token = imported
				_ = a.save(tokenResponse{
					AccessToken:  imported.AccessToken,
					RefreshToken: imported.RefreshToken,
					ExpiresIn:    int(max(0, time.Until(imported.ExpiresAt).Seconds())),
					Scope:        imported.Scopes,
				})
			}
		}
		a.loaded = true
	}

	if a.token.RefreshToken != "" && !hasScopes(a.token.Scopes) {
		if err := a.authorize(ctx); err != nil {
			return "", err
		}
		return a.token.AccessToken, nil
	}
	if a.token.AccessToken != "" && time.Until(a.token.ExpiresAt) > 30*time.Second {
		return a.token.AccessToken, nil
	}
	if a.token.RefreshToken != "" {
		if err := a.refresh(ctx); err == nil {
			return a.token.AccessToken, nil
		} else if !errors.Is(err, errInvalidGrant) {
			return "", err
		}
		// Spotify expires refresh tokens after six months. A fresh PKCE grant is needed.
		a.token = savedToken{}
		if err := keyring.Delete(keyringService, a.clientID); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("eliminar credenciales vencidas: %w", err)
		}
	}

	if err := a.authorize(ctx); err != nil {
		return "", err
	}
	return a.token.AccessToken, nil
}

func tryImportSpotifyPlayerToken(clientID string) (savedToken, bool) {
	candidates := make([]string, 0, 2)
	if cacheDir, err := os.UserCacheDir(); err == nil {
		candidates = append(candidates, filepath.Join(cacheDir, "spotify-player", clientID+"_token.json"))
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(homeDir, ".cache", "spotify-player", clientID+"_token.json"))
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var parsed struct {
			AccessToken  string    `json:"access_token"`
			RefreshToken string    `json:"refresh_token"`
			ExpiresAt    time.Time `json:"expires_at"`
			Scope        string    `json:"scope"`
		}
		if err := json.Unmarshal(data, &parsed); err == nil && parsed.RefreshToken != "" {
			return savedToken{
				AccessToken:  parsed.AccessToken,
				RefreshToken: parsed.RefreshToken,
				ExpiresAt:    parsed.ExpiresAt,
				Scopes:       parsed.Scope,
			}, true
		}
	}
	return savedToken{}, false
}

func hasScopes(granted string) bool {
	available := make(map[string]bool)
	for _, scope := range strings.Fields(granted) {
		available[scope] = true
	}
	required := []string{
		"user-read-playback-state",
		"user-modify-playback-state",
		"playlist-read-private",
		"user-library-read",
	}
	for _, scope := range required {
		if !available[scope] {
			return false
		}
	}
	return true
}

var errInvalidGrant = errors.New("Spotify invalid_grant")

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func (a *Auth) requestToken(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("solicitar token: %w", err)
	}
	defer resp.Body.Close()
	var result tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32*1024)).Decode(&result); err != nil {
		return tokenResponse{}, fmt.Errorf("leer respuesta de autenticación: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if result.Error == "invalid_grant" {
			return tokenResponse{}, errInvalidGrant
		}
		return tokenResponse{}, fmt.Errorf("Spotify OAuth: HTTP %d (%s)", resp.StatusCode, result.Error)
	}
	if result.AccessToken == "" || result.ExpiresIn <= 0 {
		return tokenResponse{}, errors.New("Spotify devolvió un token incompleto")
	}
	return result, nil
}

func (a *Auth) save(result tokenResponse) error {
	a.token.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		a.token.RefreshToken = result.RefreshToken
	}
	a.token.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	if result.Scope != "" {
		a.token.Scopes = result.Scope
	}
	data, err := json.Marshal(a.token)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, a.clientID, string(data)); err != nil {
		return fmt.Errorf("guardar credenciales en el sistema: %w", err)
	}
	return nil
}

func (a *Auth) refresh(ctx context.Context) error {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {a.token.RefreshToken},
		"client_id":     {a.clientID},
	}
	result, err := a.requestToken(ctx, form)
	if err != nil {
		return err
	}
	return a.save(result)
}

func randomURLString(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (a *Auth) authorize(ctx context.Context) error {
	verifier, err := randomURLString(32)
	if err != nil {
		return err
	}
	state, err := randomURLString(24)
	if err != nil {
		return err
	}
	challenge := sha256.Sum256([]byte(verifier))

	listener, err := net.Listen("tcp", "127.0.0.1:8989")
	if err != nil {
		return fmt.Errorf("abrir callback OAuth en 127.0.0.1:8989: %w", err)
	}
	type callbackResult struct {
		code string
		err  error
	}
	callback := make(chan callbackResult, 1)
	var callbackOnce sync.Once
	handleOAuth := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) != 1 {
			http.Error(w, "Solicitud OAuth no válida", http.StatusBadRequest)
			return
		}
		result := callbackResult{code: r.URL.Query().Get("code")}
		if reason := r.URL.Query().Get("error"); reason != "" {
			result.err = fmt.Errorf("autorización rechazada: %s", reason)
		} else if result.code == "" {
			result.err = errors.New("Spotify no devolvió un código de autorización")
		}
		accepted := false
		callbackOnce.Do(func() { accepted = true })
		if !accepted {
			http.Error(w, "Autorización ya recibida", http.StatusConflict)
			return
		}
		fmt.Fprint(w, "SpotyGo recibió la autorización. Puedes cerrar esta pestaña.")
		_ = http.NewResponseController(w).Flush()
		callback <- result
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", handleOAuth)
	mux.HandleFunc("/callback", handleOAuth)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {a.clientID},
		"scope":                 {scopes},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
	}
	authURL := "https://accounts.spotify.com/authorize?" + params.Encode()
	fmt.Fprintln(os.Stderr, "Autoriza SpotyGo en tu navegador. Si no se abre, visita:")
	fmt.Fprintln(os.Stderr, authURL)
	openBrowser(authURL)

	deadline, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var code string
	select {
	case result := <-callback:
		if result.err != nil {
			return result.err
		}
		code = result.code
	case <-deadline.Done():
		return fmt.Errorf("esperando autorización de Spotify: %w", deadline.Err())
	}

	result, err := a.requestToken(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {a.clientID},
		"code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	if result.RefreshToken == "" {
		return errors.New("Spotify no devolvió un refresh token")
	}
	if result.Scope == "" {
		result.Scope = scopes
	}
	return a.save(result)
}

func openBrowser(target string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start() // The URL is also printed so users can open it manually.
}

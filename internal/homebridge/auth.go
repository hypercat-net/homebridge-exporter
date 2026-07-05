package homebridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	OTP      string `json:"otp,omitempty"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// AuthManager obtains and refreshes Homebridge API bearer tokens.
type AuthManager struct {
	baseURL    string
	username   string
	password   string
	otp        string
	noAuth     bool
	httpClient *http.Client

	mu    sync.Mutex
	token string
}

// NewAuthManager creates an AuthManager for the given connection settings.
func NewAuthManager(baseURL, username, password, otp string, noAuth bool, httpClient *http.Client) *AuthManager {
	return &AuthManager{
		baseURL:    trimTrailingSlash(baseURL),
		username:   username,
		password:   password,
		otp:        otp,
		noAuth:     noAuth,
		httpClient: httpClient,
	}
}

// Token returns a valid bearer token, authenticating or refreshing as needed.
func (a *AuthManager) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token != "" {
		return a.token, nil
	}

	if err := a.authenticateLocked(ctx); err != nil {
		return "", err
	}
	return a.token, nil
}

// RefreshOnUnauthorized clears the cached token and obtains a new one.
func (a *AuthManager) RefreshOnUnauthorized(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.token = ""
	return a.authenticateLocked(ctx)
}

// Refresh attempts to extend the current session before it expires.
func (a *AuthManager) Refresh(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token == "" {
		return a.authenticateLocked(ctx)
	}

	token, err := a.postForToken(ctx, "/api/auth/refresh", a.token, nil)
	if err != nil {
		a.token = ""
		return a.authenticateLocked(ctx)
	}

	a.token = token
	return nil
}

func (a *AuthManager) authenticateLocked(ctx context.Context) error {
	if a.noAuth {
		token, err := a.postForToken(ctx, "/api/auth/noauth", "", nil)
		if err != nil {
			return fmt.Errorf("noauth: %w", err)
		}
		a.token = token
		return nil
	}

	body, err := json.Marshal(loginRequest{
		Username: a.username,
		Password: a.password,
		OTP:      a.otp,
	})
	if err != nil {
		return err
	}

	token, err := a.postForToken(ctx, "/api/auth/login", "", body)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	a.token = token
	return nil
}

func (a *AuthManager) postForToken(ctx context.Context, path, bearer string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned status %d: %s", path, resp.StatusCode, string(respBody))
	}

	var tr tokenResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("%s returned empty access_token", path)
	}

	return tr.AccessToken, nil
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

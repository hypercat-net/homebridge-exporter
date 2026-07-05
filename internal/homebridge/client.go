package homebridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Accessory represents one Homebridge accessory service entry.
type Accessory struct {
	UniqueID    string                 `json:"uniqueId"`
	ServiceName string                 `json:"serviceName"`
	Type        string                 `json:"type"`
	Values      map[string]interface{} `json:"values"`
}

// Client calls the Homebridge Config UI X REST API.
type Client struct {
	baseURL     string
	auth        *AuthManager
	httpClient  *http.Client
}

// NewClient creates a Homebridge API client.
func NewClient(baseURL string, auth *AuthManager, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    trimTrailingSlash(baseURL),
		auth:       auth,
		httpClient: httpClient,
	}
}

// ListAccessories returns all accessories from GET /api/accessories.
func (c *Client) ListAccessories(ctx context.Context) ([]Accessory, error) {
	body, status, err := c.doAuthorized(ctx, http.MethodGet, "/api/accessories", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		if err := c.auth.RefreshOnUnauthorized(ctx); err != nil {
			return nil, err
		}
		body, status, err = c.doAuthorized(ctx, http.MethodGet, "/api/accessories", nil)
		if err != nil {
			return nil, err
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GET /api/accessories returned status %d: %s", status, string(body))
	}

	var accessories []Accessory
	if err := json.Unmarshal(body, &accessories); err != nil {
		return nil, fmt.Errorf("decode accessories: %w", err)
	}
	return accessories, nil
}

func (c *Client) doAuthorized(ctx context.Context, method, path string, reqBody io.Reader) ([]byte, int, error) {
	token, err := c.auth.Token(ctx)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// FilterAccessories returns accessories matching the requested unique IDs.
func FilterAccessories(all []Accessory, uniqueIDs map[string]struct{}) []Accessory {
	if len(uniqueIDs) == 0 {
		return nil
	}

	out := make([]Accessory, 0, len(uniqueIDs))
	for _, acc := range all {
		if _, ok := uniqueIDs[acc.UniqueID]; ok {
			out = append(out, acc)
		}
	}
	return out
}

// NumericCharacteristicValue extracts a float64 from an accessory values map.
func NumericCharacteristicValue(values map[string]interface{}, name string) (float64, bool) {
	raw, ok := values[name]
	if !ok || raw == nil {
		return 0, false
	}

	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

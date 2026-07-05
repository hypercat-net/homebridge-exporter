package homebridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthManagerLoginAndRefresh(t *testing.T) {
	var loginCalls, refreshCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			loginCalls++
			if r.Method != http.MethodPost {
				t.Errorf("login method = %s", r.Method)
			}
			var req loginRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Username != "admin" || req.Password != "secret" {
				t.Fatalf("unexpected login body: %+v", req)
			}
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "login-token"})
		case "/api/auth/refresh":
			refreshCalls++
			auth := r.Header.Get("Authorization")
			if auth != "Bearer login-token" {
				t.Fatalf("refresh auth = %q", auth)
			}
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "refresh-token"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	auth := NewAuthManager(srv.URL, "admin", "secret", "", false, srv.Client())

	token, err := auth.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "login-token" {
		t.Fatalf("Token() = %q, want login-token", token)
	}
	if loginCalls != 1 {
		t.Fatalf("loginCalls = %d, want 1", loginCalls)
	}

	if err := auth.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d, want 1", refreshCalls)
	}

	token, err = auth.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() after refresh error = %v", err)
	}
	if token != "refresh-token" {
		t.Fatalf("Token() after refresh = %q, want refresh-token", token)
	}
}

func TestAuthManagerNoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/noauth" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "noauth-token"})
	}))
	defer srv.Close()

	auth := NewAuthManager(srv.URL, "", "", "", true, srv.Client())
	token, err := auth.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "noauth-token" {
		t.Fatalf("Token() = %q", token)
	}
}

func TestAuthManagerRefreshOnUnauthorized(t *testing.T) {
	var loginCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" {
			http.NotFound(w, r)
			return
		}
		loginCalls++
		json.NewEncoder(w).Encode(tokenResponse{AccessToken: "new-token"})
	}))
	defer srv.Close()

	auth := NewAuthManager(srv.URL, "admin", "secret", "", false, srv.Client())
	auth.token = "stale-token"

	if err := auth.RefreshOnUnauthorized(context.Background()); err != nil {
		t.Fatalf("RefreshOnUnauthorized() error = %v", err)
	}
	if loginCalls != 1 {
		t.Fatalf("loginCalls = %d, want 1", loginCalls)
	}
	if auth.token != "new-token" {
		t.Fatalf("token = %q", auth.token)
	}
}

func TestListAccessories401Retry(t *testing.T) {
	var accessoriesCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token"})
		case "/api/accessories":
			accessoriesCalls++
			if accessoriesCalls == 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode([]Accessory{{
				UniqueID:    "abc",
				ServiceName: "Fridge",
				Type:        "TemperatureSensor",
				Values:      map[string]interface{}{"CurrentTemperature": 4.2},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	auth := NewAuthManager(srv.URL, "admin", "secret", "", false, srv.Client())
	client := NewClient(srv.URL, auth, srv.Client())

	accessories, err := client.ListAccessories(context.Background())
	if err != nil {
		t.Fatalf("ListAccessories() error = %v", err)
	}
	if accessoriesCalls != 2 {
		t.Fatalf("accessoriesCalls = %d, want 2", accessoriesCalls)
	}
	if len(accessories) != 1 || accessories[0].UniqueID != "abc" {
		t.Fatalf("unexpected accessories: %+v", accessories)
	}
}

func TestListAccessories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token"})
		case "/api/accessories":
			if r.Header.Get("Authorization") != "Bearer token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode([]Accessory{{
				UniqueID:    "abc",
				ServiceName: "Fridge",
				Type:        "TemperatureSensor",
				Values:      map[string]interface{}{"CurrentTemperature": 4.2},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	auth := NewAuthManager(srv.URL, "admin", "secret", "", false, srv.Client())
	client := NewClient(srv.URL, auth, srv.Client())

	accessories, err := client.ListAccessories(context.Background())
	if err != nil {
		t.Fatalf("ListAccessories() error = %v", err)
	}
	if len(accessories) != 1 || accessories[0].UniqueID != "abc" {
		t.Fatalf("unexpected accessories: %+v", accessories)
	}
}

func TestFilterAccessoriesAndNumericValue(t *testing.T) {
	all := []Accessory{
		{UniqueID: "a", Values: map[string]interface{}{"CurrentTemperature": 1.5}},
		{UniqueID: "b", Values: map[string]interface{}{"On": true}},
		{UniqueID: "c", Values: map[string]interface{}{"CurrentTemperature": json.Number("2.5")}},
	}

	filtered := FilterAccessories(all, map[string]struct{}{"a": {}, "c": {}})
	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d", len(filtered))
	}

	v, ok := NumericCharacteristicValue(all[0].Values, "CurrentTemperature")
	if !ok || v != 1.5 {
		t.Fatalf("numeric value = %v, ok = %v", v, ok)
	}

	_, ok = NumericCharacteristicValue(all[1].Values, "On")
	if ok {
		t.Fatal("expected boolean On to be skipped")
	}

	v, ok = NumericCharacteristicValue(all[2].Values, "CurrentTemperature")
	if !ok || v != 2.5 {
		t.Fatalf("json.Number value = %v, ok = %v", v, ok)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// helper to set stub transport
func withStubTransport(rt http.RoundTripper, fn func()) {
	oldClient := httpClient
	httpClient = &http.Client{Transport: rt}
	fn()
	httpClient = oldClient
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "sk-or-12345678901234567890123456789012")
	os.Setenv("OPENROUTER_MODEL", "openai/gpt-4o")
	cfg := loadConfig()
	if cfg.apiKey != "sk-or-12345678901234567890123456789012" {
		t.Fatalf("unexpected api key: %s", cfg.apiKey)
	}
	if cfg.model != "openai/gpt-4o" {
		t.Fatalf("unexpected model: %s", cfg.model)
	}
}

func TestLoadConfigFromDotEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/.env", []byte("OPENROUTER_API_KEY=sk-or-abcdefghijklmnopqrstuvwxyz123456\nOPENROUTER_MODEL=openai/gpt-4o\n"), 0644)
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_MODEL")
	oldwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldwd)
	cfg := loadConfig()
	if cfg.apiKey != "sk-or-abcdefghijklmnopqrstuvwxyz123456" {
		t.Fatalf("failed to load api key from .env: %s", cfg.apiKey)
	}
	if cfg.model != "openai/gpt-4o" {
		t.Fatalf("failed to load model from .env: %s", cfg.model)
	}
}

func TestHealthHandler(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "sk-or-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	os.Setenv("OPENROUTER_MODEL", "openai/gpt-4o")
	loadConfig()
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != openRouterEndpoint+"/models" {
			t.Fatalf("unexpected url: %s", req.URL)
		}
		resp := httptest.NewRecorder()
		resp.WriteHeader(http.StatusOK)
		resp.Body.WriteString(`{}`)
		return resp.Result(), nil
	})
	withStubTransport(rt, func() {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/health", nil)
		healthHandler(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})
}

func TestModelsEndpoint(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "sk-or-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	os.Setenv("OPENROUTER_MODEL", "openai/gpt-4o")
	loadConfig()
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/models" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		resp := httptest.NewRecorder()
		resp.WriteHeader(http.StatusOK)
		resp.Body.WriteString(`{"data":[]}`)
		return resp.Result(), nil
	})
	withStubTransport(rt, func() {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/models", nil)
		proxyHandler(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})
}

func TestConfigEndpoints(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "sk-or-cccccccccccccccccccccccccccccccc")
	os.Setenv("OPENROUTER_MODEL", "openai/gpt-4o")
	loadConfig()
	// GET
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/config", nil)
	proxyHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 get, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["model"] != activeConfig.model {
		t.Fatalf("unexpected model in get: %s", resp["model"])
	}
	// POST update
	body := bytes.NewBufferString(`{"model":"google/gemini-pro"}`)
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/config", body)
	proxyHandler(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 post, got %d", rr2.Code)
	}
	json.NewDecoder(rr2.Body).Decode(&resp)
	if resp["model"] != "google/gemini-pro" {
		t.Fatalf("model not updated: %s", resp["model"])
	}
}

func TestChatCompletions(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "sk-or-dddddddddddddddddddddddddddddddd")
	os.Setenv("OPENROUTER_MODEL", "openai/gpt-4o")
	loadConfig()
	activeConfig.endpoint = "https://openrouter.ai/api/v1"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		body, _ := io.ReadAll(req.Body)
		if !bytes.Contains(body, []byte(activeConfig.model)) {
			t.Fatalf("proxy did not replace model: %s", string(body))
		}
		resp := httptest.NewRecorder()
		resp.WriteHeader(http.StatusOK)
		resp.Body.WriteString(`{"id":"1","choices":[{"message":{"content":"hi"}}]}`)
		return resp.Result(), nil
	})
	withStubTransport(rt, func() {
		reqBody := bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
		req := httptest.NewRequest("POST", "/v1/chat/completions", reqBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-test-123456")
		rr := httptest.NewRecorder()
		proxyHandler(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})
}

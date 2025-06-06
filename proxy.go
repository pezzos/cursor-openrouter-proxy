package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/joho/godotenv"
	"github.com/klauspost/compress/gzip"
	"golang.org/x/net/http2"
)

const (
	openRouterEndpoint = "https://openrouter.ai/api/v1"
	openRouterModel    = "openai/gpt-4o"
	cursorMockedModel  = "gpt-4o"
)

var (
	openRouterAPIKey string
)

// Configuration structure
type Config struct {
	endpoint string
	model    string
	apiKey   string
}

var activeConfig Config

// Global HTTP client with optimized settings
var httpClient = &http.Client{
	Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLS:   nil,
		// Optimize connection pooling
		ReadIdleTimeout:  30 * time.Second,
		PingTimeout:      10 * time.Second,
		WriteByteTimeout: 15 * time.Second,
	},
	Timeout: 5 * time.Minute,
}

var (
	// Buffer pools for various sizes
	smallBufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	largeBufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	// Debug mode flag
	debugMode = os.Getenv("DEBUG") == "true"
)

func getBuffer(size int) *bytes.Buffer {
	var buf *bytes.Buffer
	if size < 1024 {
		buf = smallBufferPool.Get().(*bytes.Buffer)
	} else {
		buf = largeBufferPool.Get().(*bytes.Buffer)
	}
	buf.Reset()
	return buf
}

func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() < 1024 {
		smallBufferPool.Put(buf)
	} else {
		largeBufferPool.Put(buf)
	}
}

func init() {
	// Check environment variables first
	openRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
	defaultModel := os.Getenv("OPENROUTER_MODEL")

	// If key or model is missing, try loading from .env file
	if openRouterAPIKey == "" || defaultModel == "" {
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: .env file not found or error loading it: %v", err)
		}
		if openRouterAPIKey == "" {
			openRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
		}
		if defaultModel == "" {
			defaultModel = os.Getenv("OPENROUTER_MODEL")
		}
	}

	// Ensure API key is provided and has correct format
	if !strings.HasPrefix(openRouterAPIKey, "sk-or-") {
		log.Fatal("OPENROUTER_API_KEY must start with 'sk-or-'")
	}
	if len(openRouterAPIKey) < 32 {
		log.Fatal("OPENROUTER_API_KEY seems too short to be valid")
	}

	// Validate or fallback to default model
	if defaultModel == "" {
		defaultModel = openRouterModel
	} else if !strings.Contains(defaultModel, "/") {
		// If model doesn't contain a provider prefix, fails
		log.Fatalf("Invalid model: %s. Must contain a provider prefix (e.g. openai/gpt-4o)", defaultModel)
	}

	// Configure the active endpoint and model
	activeConfig = Config{
		endpoint: openRouterEndpoint,
		model:    defaultModel,
		apiKey:   openRouterAPIKey,
	}

	log.Printf("Initialized Cursor-OpenRouter proxy with model: %s using endpoint: %s", activeConfig.model, activeConfig.endpoint)
}

// Models response structure
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAI compatible request structure
type ChatRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Stream      bool        `json:"stream"`
	Functions   []Function  `json:"functions,omitempty"`
	Tools       []Tool      `json:"tools,omitempty"`
	ToolChoice  interface{} `json:"tool_choice,omitempty"`
	Temperature *float64    `json:"temperature,omitempty"`
	MaxTokens   *int        `json:"max_tokens,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type Function struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func convertToolChoice(choice interface{}) string {
	if choice == nil {
		return ""
	}

	// If string "auto" or "none"
	if str, ok := choice.(string); ok {
		switch str {
		case "auto", "none":
			return str
		}
	}

	// Try to parse as map for function call
	if choiceMap, ok := choice.(map[string]interface{}); ok {
		if choiceMap["type"] == "function" {
			return "auto" // DeepSeek doesn't support specific function selection, default to auto
		}
	}

	return ""
}

func convertMessages(messages []Message) []Message {
	converted := make([]Message, len(messages))
	for i, msg := range messages {
		log.Printf("Converting message %d - Role: %s", i, msg.Role)
		converted[i] = msg

		// Handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			log.Printf("Processing assistant message with %d tool calls", len(msg.ToolCalls))
			// DeepSeek expects tool_calls in a specific format
			toolCalls := make([]ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				toolCalls[j] = ToolCall{
					ID:       tc.ID,
					Type:     "function",
					Function: tc.Function,
				}
				log.Printf("Tool call %d - ID: %s, Function: %s", j, tc.ID, tc.Function.Name)
			}
			converted[i].ToolCalls = toolCalls
		}

		// Handle function response messages
		if msg.Role == "function" {
			log.Printf("Converting function response to tool response")
			// Convert to tool response format
			converted[i].Role = "tool"
		}
	}

	// Log the final converted messages
	for i, msg := range converted {
		log.Printf("Final message %d - Role: %s, Content: %s", i, msg.Role, truncateString(msg.Content, 50))
		if len(msg.ToolCalls) > 0 {
			log.Printf("Message %d has %d tool calls", i, len(msg.ToolCalls))
		}
	}

	return converted
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Convert to OpenRouter request format
type OpenRouterRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
}

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	// Add health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Test OpenRouter connection
		req, err := http.NewRequest("GET", openRouterEndpoint+"/models", nil)
		if err != nil {
			log.Printf("Error creating health check request: %v", err)
			http.Error(w, "Error creating request", http.StatusInternalServerError)
			return
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", activeConfig.apiKey))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://github.com/pezzos/cursor-proxy")
		req.Header.Set("X-Title", "Cursor Proxy")
		req.Header.Set("OpenAI-Organization", "cursor-proxy")

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Health check failed: %v", err)
			http.Error(w, "Connection failed", http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Health check failed with status %d: %s", resp.StatusCode, string(body))
			http.Error(w, fmt.Sprintf("OpenRouter returned %d", resp.StatusCode), resp.StatusCode)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "ok",
			"endpoint": openRouterEndpoint,
		})
	})

	server := &http.Server{
		Addr:    ":9000",
		Handler: http.HandlerFunc(proxyHandler),
	}

	// Enable HTTP/2 support
	http2.ConfigureServer(server, &http2.Server{})

	log.Printf("Starting proxy server on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func enableCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func maskAPIKey(key string) string {
	if len(key) <= 12 {
		return "***"
	}
	return key[:6] + "..." + key[len(key)-6:]
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	debugLog("Received request: %s %s", r.Method, r.URL.Path)

	if r.Method == "OPTIONS" {
		enableCors(w)
		return
	}

	enableCors(w)

	// Handle /v1/config endpoint for GET
	if r.URL.Path == "/v1/config" && r.Method == "GET" {
		handleGetConfigRequest(w, r)
		return
	}

	// Handle /v1/models endpoint
	if r.URL.Path == "/v1/models" && r.Method == "GET" {
		handleGetModelsRequest(w)
		return
	}

	// Handle /v1/config endpoint for POST
	if r.URL.Path == "/v1/config" && r.Method == "POST" {
		handleConfigRequest(w, r)
		return
	}

	// Only handle API requests with /v1/ prefix
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		log.Printf("Invalid path: %s", r.URL.Path)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Validate API key
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		debugLog("Missing or invalid Authorization header")
		http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}

	// Only check that the key has a valid format (sk-* or Bearer *)
	userAPIKey := strings.TrimPrefix(authHeader, "Bearer ")
	if !strings.HasPrefix(strings.TrimSpace(userAPIKey), "sk-") {
		log.Printf("Invalid API key format")
		http.Error(w, "Invalid API key format", http.StatusUnauthorized)
		return
	}

	// Read and log request body for debugging
	var chatReq ChatRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		debugLog("Error reading request body: %v", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	if err := json.Unmarshal(body, &chatReq); err != nil {
		log.Printf("Error parsing request JSON: %v", err)
		log.Printf("Raw request body: %s", string(body))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("Parsed request: %+v", chatReq)

	// Replace gpt-4o model with the appropriate model
	if chatReq.Model == cursorMockedModel {
		log.Printf("Converting gpt-4o to configured model: %s (endpoint: %s)", activeConfig.model, activeConfig.endpoint)
		chatReq.Model = activeConfig.model
		log.Printf("Model converted to: %s", activeConfig.model)
	} else {
		log.Printf("Unsupported model requested: %s", chatReq.Model)
		http.Error(w, fmt.Sprintf("Model %s not supported. Use %s instead.", chatReq.Model, cursorMockedModel), http.StatusBadRequest)
		return
	}

	// Convert to OpenRouter request format with model-specific adjustments
	openRouterReq := OpenRouterRequest{
		Model:    activeConfig.model,
		Messages: convertMessages(chatReq.Messages),
		Stream:   chatReq.Stream,
	}

	// Model-specific adjustments
	switch {
	case strings.HasPrefix(activeConfig.model, "mistralai/"):
		if chatReq.Temperature != nil {
			temp := *chatReq.Temperature
			if temp > 1.0 {
				temp = 1.0
			}
			openRouterReq.Temperature = temp
		}
	case strings.HasPrefix(activeConfig.model, "google/"):
		if chatReq.Temperature != nil {
			temp := *chatReq.Temperature
			if temp > 1.0 {
				temp = 1.0
			}
			openRouterReq.Temperature = temp
		}
		if chatReq.MaxTokens != nil {
			openRouterReq.MaxTokens = *chatReq.MaxTokens
		}
	default:
		if chatReq.Temperature != nil {
			openRouterReq.Temperature = *chatReq.Temperature
		}
		if chatReq.MaxTokens != nil {
			openRouterReq.MaxTokens = *chatReq.MaxTokens
		}
	}

	// Handle tools/functions
	if len(chatReq.Tools) > 0 {
		openRouterReq.Tools = chatReq.Tools
		if tc := convertToolChoice(chatReq.ToolChoice); tc != "" {
			openRouterReq.ToolChoice = tc
		}
	} else if len(chatReq.Functions) > 0 {
		tools := make([]Tool, len(chatReq.Functions))
		for i, fn := range chatReq.Functions {
			tools[i] = Tool{
				Type:     "function",
				Function: fn,
			}
		}
		openRouterReq.Tools = tools
		if tc := convertToolChoice(chatReq.ToolChoice); tc != "" {
			openRouterReq.ToolChoice = tc
		}
	}

	// Create new request body
	modifiedBody, err := json.Marshal(openRouterReq)
	if err != nil {
		log.Printf("Error creating modified request body: %v", err)
		http.Error(w, "Error creating modified request", http.StatusInternalServerError)
		return
	}

	log.Printf("Modified request body: %s", string(modifiedBody))

	// Create the proxy request to OpenRouter
	targetURL := activeConfig.endpoint
	if !strings.HasSuffix(targetURL, "/") {
		targetURL += "/"
	}
	targetURL += strings.TrimPrefix(r.URL.Path, "/v1/")
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		log.Printf("Error creating proxy request: %v", err)
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// Set common headers
	proxyReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", activeConfig.apiKey))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Accept", "application/json")
	proxyReq.Header.Set("User-Agent", "cursor-proxy/1.0")
	proxyReq.Header.Set("HTTP-Referer", "https://github.com/pezzos/cursor-proxy")
	proxyReq.Header.Set("X-Title", "Cursor Proxy")
	proxyReq.Header.Set("OpenAI-Organization", "cursor-proxy")

	// Model-specific headers
	switch {
	case strings.HasPrefix(activeConfig.model, "mistralai/"):
		proxyReq.Header.Set("X-Model-Provider", "mistral")
	case strings.HasPrefix(activeConfig.model, "google/"):
		proxyReq.Header.Set("X-Model-Provider", "google")
	}

	// Remove problematic headers
	proxyReq.Header.Del("X-Forwarded-For")
	proxyReq.Header.Del("X-Forwarded-Host")
	proxyReq.Header.Del("X-Forwarded-Port")
	proxyReq.Header.Del("X-Forwarded-Proto")
	proxyReq.Header.Del("X-Forwarded-Server")
	proxyReq.Header.Del("X-Real-Ip")

	if chatReq.Stream {
		proxyReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("Error forwarding request: %v", err)
		http.Error(w, "Error forwarding request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("OpenRouter response status: %d", resp.StatusCode)
	log.Printf("OpenRouter response headers: %v", resp.Header)

	// Handle error responses with better error handling
	if resp.StatusCode >= 400 {
		respBody, err := readResponse(resp)
		if err != nil {
			log.Printf("Error reading error response: %v", err)
			http.Error(w, "Error reading response", http.StatusInternalServerError)
			return
		}

		log.Printf("Error response body: %s", string(respBody))

		// Try to parse the error response
		var openRouterErr struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    int    `json:"code"`
			} `json:"error"`
		}

		if err := json.Unmarshal(respBody, &openRouterErr); err != nil {
			// If we can't parse the error, return the raw response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(respBody)
			return
		}

		// Return a properly formatted error response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": openRouterErr.Error.Message,
				"type":    openRouterErr.Error.Type,
				"code":    openRouterErr.Error.Code,
			},
		})
		return
	}

	// Handle streaming response
	if chatReq.Stream {
		handleStreamingResponse(w, r, resp)
		return
	}

	// Handle regular response
	handleRegularResponse(w, resp)
}

func handleStreamingResponse(w http.ResponseWriter, r *http.Request, resp *http.Response) {
	debugLog("Starting streaming response handling")
	debugLog("Response status: %d", resp.StatusCode)
	debugLog("Response headers: %+v", resp.Header)

	// Set headers for streaming response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	// Create a buffered reader for the response body
	reader := bufio.NewReader(resp.Body)

	// Create a context with cancel for cleanup
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Start a goroutine to send heartbeats
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Send a heartbeat comment
				if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
					log.Printf("Error sending heartbeat: %v", err)
					cancel()
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled, ending stream")
			return
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					continue
				}
				log.Printf("Error reading stream: %v", err)
				cancel()
				return
			}

			// Skip empty lines
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}

			// Write the line to the response
			if _, err := w.Write(line); err != nil {
				log.Printf("Error writing to response: %v", err)
				cancel()
				return
			}

			// Flush the response writer
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			} else {
				log.Printf("Warning: ResponseWriter does not support Flush")
			}
		}
	}
}

func handleRegularResponse(w http.ResponseWriter, resp *http.Response) {
	debugLog("Handling regular (non-streaming) response")
	debugLog("Response status: %d", resp.StatusCode)
	debugLog("Response headers: %+v", resp.Header)

	// Read and log response body
	body, err := readResponse(resp)
	if err != nil {
		debugLog("Error reading response: %v", err)
		http.Error(w, "Error reading response from upstream", http.StatusInternalServerError)
		return
	}

	debugLog("Original response body: %s", string(body))

	// Parse the OpenRouter response
	var openRouterResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		debugLog("Error parsing OpenRouter response: %v", err)
		debugLog("Response body that failed to parse: %s", string(body))
		http.Error(w, fmt.Sprintf("Error parsing response: %v", err), http.StatusInternalServerError)
		return
	}

	// Check for OpenRouter error
	if openRouterResp.Error != nil {
		debugLog("OpenRouter returned error: %+v", openRouterResp.Error)
		http.Error(w, openRouterResp.Error.Message, openRouterResp.Error.Code)
		return
	}

	// Convert to OpenAI format
	openAIResp := struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}{
		ID:      openRouterResp.ID,
		Object:  "chat.completion",
		Created: openRouterResp.Created,
		Model:   cursorMockedModel,
		Usage:   openRouterResp.Usage,
	}

	openAIResp.Choices = make([]struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	}, len(openRouterResp.Choices))

	for i, choice := range openRouterResp.Choices {
		openAIResp.Choices[i] = struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		}{
			Index:        choice.Index,
			Message:      choice.Message,
			FinishReason: choice.FinishReason,
		}

		if len(choice.Message.ToolCalls) > 0 {
			debugLog("Processing %d tool calls in choice %d", len(choice.Message.ToolCalls), i)
			for j, tc := range choice.Message.ToolCalls {
				debugLog("Tool call %d: %+v", j, tc)
				if tc.Function.Name == "" {
					debugLog("Warning: Empty function name in tool call %d", j)
					continue
				}
				openAIResp.Choices[i].Message.ToolCalls = append(openAIResp.Choices[i].Message.ToolCalls, tc)
			}
		}
	}

	modifiedBody, err := json.Marshal(openAIResp)
	if err != nil {
		debugLog("Error creating modified response: %v", err)
		http.Error(w, fmt.Sprintf("Error creating response: %v", err), http.StatusInternalServerError)
		return
	}

	debugLog("Modified response body: %s", string(modifiedBody))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(modifiedBody)
	debugLog("Modified response sent successfully")
}

func copyHeaders(dst, src http.Header) {
	skipHeaders := map[string]bool{
		"Content-Length":    true,
		"Content-Encoding":  true,
		"Transfer-Encoding": true,
		"Connection":        true,
	}

	for k, vv := range src {
		if !skipHeaders[k] {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func handleModelsRequest(w http.ResponseWriter) {
	debugLog("Handling models request")
	response := ModelsResponse{
		Object: "list",
		Data: []Model{
			{
				ID:      "gpt-4o",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "openai",
			},
			{
				ID:      "deepseek-chat",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "deepseek",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	debugLog("Models response sent successfully")
}

func readResponse(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	contentEncoding := resp.Header.Get("Content-Encoding")
	log.Printf("Response Content-Encoding: %s", contentEncoding)

	switch contentEncoding {
	case "gzip":
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
		log.Printf("Using gzip decompression")
	case "br":
		reader = brotli.NewReader(resp.Body)
		log.Printf("Using brotli decompression")
	default:
		log.Printf("No compression detected")
	}

	buf := getBuffer(int(resp.ContentLength))
	defer putBuffer(buf)

	n, err := io.Copy(buf, reader)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}
	log.Printf("Read %d bytes from response", n)

	return buf.Bytes(), nil
}

func handleConfigRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config struct {
		Model string `json:"model"`
	}

	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if config.Model == "" {
		http.Error(w, "Model is required", http.StatusBadRequest)
		return
	}

	activeConfig.model = config.Model
	log.Printf("Updated model to: %s", activeConfig.model)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"model":  activeConfig.model,
	})
}

func handleGetConfigRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"model": activeConfig.model,
	})
}

func handleGetModelsRequest(w http.ResponseWriter) {
	// Manually create the request for the models endpoint for future header customization
	req, err := http.NewRequest(http.MethodGet, openRouterEndpoint+"/models", nil)
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, "Error fetching models", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to fetch models", resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

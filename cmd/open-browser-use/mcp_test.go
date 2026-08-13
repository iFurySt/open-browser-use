package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ifuryst/open-browser-use/internal/wire"
)

func TestMCPInitializeAndListTools(t *testing.T) {
	server := newMCPServer(socketOptions{socketDir: t.TempDir(), timeout: time.Second})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		"",
	}, "\n")
	var output bytes.Buffer

	if err := server.serve(strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}

	responses := decodeMCPResponses(t, output.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected initialize and tools/list responses, got %#v", responses)
	}
	initializeResult, _ := responses[0]["result"].(map[string]any)
	if initializeResult["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("expected protocol version %q, got %#v", mcpProtocolVersion, initializeResult["protocolVersion"])
	}
	capabilities, _ := initializeResult["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"].(map[string]any); !ok {
		t.Fatalf("expected tools capability, got %#v", capabilities)
	}

	listResult, _ := responses[1]["result"].(map[string]any)
	tools, _ := listResult["tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("expected tools/list to return tools, got %#v", listResult)
	}
	names := map[string]bool{}
	for _, rawTool := range tools {
		tool, _ := rawTool.(map[string]any)
		name, _ := tool["name"].(string)
		names[name] = true
		if _, ok := tool["inputSchema"].(map[string]any); !ok {
			t.Fatalf("expected tool %q to include inputSchema, got %#v", name, tool)
		}
	}
	for _, name := range []string{
		"user_tabs", "open_tab", "claim_status", "active_tab", "scroll", "extract",
		"wait_for", "click_element", "type_into", "select_option", "screenshot",
		"focus_state", "close_tab", "reconcile_operation", "cdp", "run_action_plan",
	} {
		if !names[name] {
			t.Fatalf("expected MCP tools to include %q, got %#v", name, names)
		}
	}
}

func TestMCPToolCallInvokesBrowserSocket(t *testing.T) {
	socketDir, err := os.MkdirTemp("/tmp", "obu-mcp-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	socketPath := filepath.Join(socketDir, "browser.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requests := make(chan map[string]any, 1)
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		var request map[string]any
		if err := wire.ReadJSON(conn, &request); err != nil {
			serverDone <- err
			return
		}
		requests <- request
		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"result": []any{
				map[string]any{"id": 321, "title": "Example", "url": "https://example.com"},
			},
		}
		if err := wire.WriteJSON(conn, response); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	server := newMCPServer(socketOptions{socketPath: socketPath, timeout: time.Second})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"call-1","method":"tools/call","params":{"name":"user_tabs","arguments":{}}}`,
		"",
	}, "\n")
	var output bytes.Buffer

	if err := server.serve(strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	browserRequest := <-requests
	if browserRequest["method"] != "getUserTabs" {
		t.Fatalf("expected getUserTabs browser request, got %#v", browserRequest["method"])
	}
	params, _ := browserRequest["params"].(map[string]any)
	if params["session_id"] == "" || params["turn_id"] == "" {
		t.Fatalf("expected MCP tool call to include session and turn ids, got %#v", params)
	}

	responses := decodeMCPResponses(t, output.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected initialize and tools/call responses, got %#v", responses)
	}
	callResult, _ := responses[1]["result"].(map[string]any)
	if callResult["isError"] != false {
		t.Fatalf("expected successful tool result, got %#v", callResult)
	}
	if _, ok := callResult["structuredContent"].(map[string]any); !ok {
		t.Fatalf("expected structuredContent with browser response, got %#v", callResult)
	}
	content, _ := callResult["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected text content mirror, got %#v", callResult["content"])
	}
}

func TestMCPExtractUsesBackgroundRuntimeEvaluation(t *testing.T) {
	socketDir, err := os.MkdirTemp("/tmp", "obu-mcp-extract-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	socketPath := filepath.Join(socketDir, "browser.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requests := make(chan map[string]any, 2)
	serverDone := make(chan error, 1)
	go func() {
		for index := 0; index < 2; index++ {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				serverDone <- acceptErr
				return
			}
			var request map[string]any
			if readErr := wire.ReadJSON(conn, &request); readErr != nil {
				_ = conn.Close()
				serverDone <- readErr
				return
			}
			requests <- request
			result := map[string]any{}
			if request["method"] == "executeCdp" {
				result = map[string]any{"result": map[string]any{"value": map[string]any{
					"selector": ".comment", "items": []any{map[string]any{"text": "hello"}},
				}}}
			}
			if writeErr := wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result}); writeErr != nil {
				_ = conn.Close()
				serverDone <- writeErr
				return
			}
			_ = conn.Close()
		}
		serverDone <- nil
	}()

	server := newMCPServer(socketOptions{socketPath: socketPath, timeout: time.Second})
	result, err := server.runTool("extract", map[string]any{
		"tab_id": 77, "selector": ".comment", "max_items": 4, "max_chars": 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected extract result")
	}
	attachRequest := <-requests
	cdpRequest := <-requests
	if attachRequest["method"] != "attach" || cdpRequest["method"] != "executeCdp" {
		t.Fatalf("expected attach then executeCdp, got %#v then %#v", attachRequest["method"], cdpRequest["method"])
	}
	params, _ := cdpRequest["params"].(map[string]any)
	if params["method"] != "Runtime.evaluate" {
		t.Fatalf("expected Runtime.evaluate, got %#v", params["method"])
	}
	commandParams, _ := params["commandParams"].(map[string]any)
	expression, _ := commandParams["expression"].(string)
	if !strings.Contains(expression, `document.querySelectorAll(".comment")`) {
		t.Fatalf("expected bounded selector extraction expression, got %q", expression)
	}
	if strings.Contains(expression, "bringToFront") {
		t.Fatalf("extract must not activate Chrome, got %q", expression)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestMCPScreenshotReturnsImageContentWithoutTextDuplication(t *testing.T) {
	result, err := mcpScreenshotResult(map[string]any{
		"result": map[string]any{"data": "aW1hZ2U="},
	}, "png")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := result["content"].([]map[string]any)
	if len(content) != 1 || content[0]["type"] != "image" || content[0]["mimeType"] != "image/png" {
		t.Fatalf("expected MCP image content, got %#v", result)
	}
	if _, hasText := content[0]["text"]; hasText {
		t.Fatalf("screenshot must not duplicate base64 into text content: %#v", content[0])
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured["format"] != "png" {
		t.Fatalf("expected structured screenshot metadata, got %#v", structured)
	}
}

func decodeMCPResponses(t *testing.T, payload []byte) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("failed to decode MCP response %q: %v", line, err)
		}
		responses = append(responses, response)
	}
	return responses
}

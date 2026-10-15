//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifuryst/open-browser-use/internal/wire"
	"golang.org/x/sys/unix"
)

func iabMCPFixture(t *testing.T, handler func(map[string]any) map[string]any) (int, <-chan struct{}) {
	t.Helper()
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(pair[0]), "iab-mcp-test")
	conn, err := net.FileConn(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "method": "iab.bootstrap", "params": iabBootstrap{
			AuthVersion: 1, Transport: "private", InstanceID: "fixture", ScopeID: "mcp-task", GrantID: "grant", Secret: strings.Repeat("A", 43),
		}})
		for {
			var request map[string]any
			if wire.ReadJSON(conn, &request) != nil {
				return
			}
			result := map[string]any{"jsonrpc": "2.0", "id": request["id"]}
			if request["method"] == "auth.connect" {
				result["result"] = map[string]any{"authenticated": true, "scopeId": "mcp-task"}
			} else {
				for key, value := range handler(request) {
					result[key] = value
				}
				wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "method": "onTabUpdated", "params": map[string]any{}})
			}
			if wire.WriteJSON(conn, result) != nil {
				return
			}
		}
	}()
	return pair[1], done
}

func TestIabMCPCommandLifecycleAndExplicitTools(t *testing.T) {
	var calls []map[string]any
	fd, done := iabMCPFixture(t, func(request map[string]any) map[string]any {
		calls = append(calls, request)
		var result any = map[string]any{}
		switch request["method"] {
		case "createTab":
			result = map[string]any{"id": 7}
		case "getTabs":
			result = []any{map[string]any{"id": 7}, map[string]any{"id": 8}}
		case "executeCdp":
			params := request["params"].(map[string]any)
			if params["method"] == "Page.captureScreenshot" {
				result = map[string]any{"data": "cG5n"}
			}
		}
		return map[string]any{"result": result}
	})
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"open_tab","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"tabs"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"navigate","arguments":{"tab_id":7,"url":"https://example.com"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"screenshot","arguments":{"tab_id":8}}}`,
		"",
	}, "\n")
	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"iab", "mcp", "--channel-fd", strconv.Itoa(fd), "--turn-id", "mcp-turn"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	<-done // EOF closed the private channel; no implicit cleanup RPC was sent.
	responses := decodeMCPResponses(t, output.Bytes())
	if len(responses) != 6 {
		t.Fatalf("unexpected responses: %s", output.String())
	}
	initialize := responses[0]["result"].(map[string]any)
	if initialize["serverInfo"].(map[string]any)["name"] != "open-browser-use-iab" {
		t.Fatal(initialize)
	}
	list := responses[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, raw := range list {
		names[raw.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{"tabs", "open_tab", "close_tab", "navigate", "snapshot", "screenshot", "click", "type_text", "cdp", "page_info", "info", "ping"} {
		if !names[name] {
			t.Fatalf("missing tool %s", name)
		}
	}
	for _, name := range []string{"history", "user_tabs", "claim_tab", "call", "run_action_plan", "finalize_tabs"} {
		if names[name] {
			t.Fatalf("exposed unsupported tool %s", name)
		}
	}
	for _, request := range calls {
		params := request["params"].(map[string]any)
		if params["session_id"] != "mcp-task" || params["turn_id"] != "mcp-turn" {
			t.Fatal("scope/turn not fixed")
		}
	}
	structured := responses[3]["result"].(map[string]any)["structuredContent"].(map[string]any)
	if len(structured["result"].([]any)) != 2 {
		t.Fatal("array result was not wrapped as an object")
	}
	image := responses[5]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	if image["type"] != "image" || image["mimeType"] != "image/png" || image["data"] != "cG5n" {
		t.Fatal(image)
	}
	if strings.Contains(output.String(), strings.Repeat("A", 43)) {
		t.Fatal("bootstrap secret leaked")
	}
}

func TestIabMCPRejectsInvalidArgumentsBeforeDispatch(t *testing.T) {
	s := &iabMCP{} // Any accidental browser dispatch would fail this test.
	for _, item := range []struct {
		name string
		args map[string]any
	}{
		{"user_tabs", map[string]any{}},
		{"tabs", map[string]any{"session_id": "other"}},
		{"open_tab", map[string]any{"url": true}},
		{"navigate", map[string]any{"url": "https://example.com"}},
		{"navigate", map[string]any{"tab_id": 0, "url": "https://example.com"}},
		{"navigate", map[string]any{"tab_id": 1.5, "url": "https://example.com"}},
		{"navigate", map[string]any{"tab_id": 1}},
		{"cdp", map[string]any{"tab_id": 1, "method": "Runtime.evaluate", "params": nil}},
		{"click", map[string]any{"tab_id": 1, "x": -1, "y": 0}},
		{"click", map[string]any{"tab_id": 1, "x": 2}},
		{"type_text", map[string]any{"tab_id": 1, "text": 42}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := s.runTool(item.name, item.args); err == nil {
				t.Fatal("accepted invalid arguments")
			}
		})
	}
}

func TestIabMCPFailedAuthProducesNoProtocolOutput(t *testing.T) {
	fd, _ := privateIabFixture(t, "task", false)
	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetIn(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"))
	cmd.SetOut(&output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"iab", "mcp", "--channel-fd", strconv.Itoa(fd)})
	if err := cmd.Execute(); err == nil {
		t.Fatal("accepted failed authentication")
	}
	if output.Len() != 0 {
		t.Fatal("MCP served before authenticating")
	}
}

func TestIabMCPBrowserErrorsAreToolErrorsAndConnectionRemainsUsable(t *testing.T) {
	fd, done := iabMCPFixture(t, func(request map[string]any) map[string]any {
		if request["method"] == "attach" {
			return map[string]any{"error": map[string]any{"code": -32000, "message": "not owned"}}
		}
		return map[string]any{"result": []any{}}
	})
	client, err := connectIab(fd, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s := newIabMCPServer(client, "turn")
	bad, _ := s.callTool(json.RawMessage(`{"name":"snapshot","arguments":{"tab_id":999}}`))
	if bad["isError"] != true {
		t.Fatal("browser rejection was not an MCP tool error")
	}
	good, _ := s.callTool(json.RawMessage(`{"name":"tabs"}`))
	if good["isError"] != false {
		t.Fatal("connection lost after browser rejection")
	}
	client.conn.Close()
	<-done
}

func TestIabRequestDeadlineResetsAfterIdle(t *testing.T) {
	fd, done := iabMCPFixture(t, func(_ map[string]any) map[string]any { return map[string]any{"result": "pong"} })
	client, err := connectIab(fd, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.timeout = 100 * time.Millisecond
	if _, err = client.call("ping", map[string]any{}, "turn"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err = client.call("ping", map[string]any{}, "turn"); err != nil {
		t.Fatalf("idle session retained stale deadline: %v", err)
	}
	client.conn.Close()
	<-done
}

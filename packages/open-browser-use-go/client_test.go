package obu

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ifuryst/open-browser-use/internal/wire"
)

func TestEncodeFrame(t *testing.T) {
	frame, err := EncodeFrame(map[string]any{"id": 1, "method": "getInfo"})
	if err != nil {
		t.Fatal(err)
	}
	length := wire.NativeEndian().Uint32(frame[:4])
	if int(length) != len(frame)-4 {
		t.Fatalf("expected payload length %d, got %d", len(frame)-4, length)
	}
	var payload map[string]any
	if err := json.Unmarshal(frame[4:], &payload); err != nil {
		t.Fatal(err)
	}
	if payload["method"] != "getInfo" {
		t.Fatalf("expected getInfo payload, got %#v", payload)
	}
}

func TestRequestRoundTrip(t *testing.T) {
	socketPath, closeServer := startSDKServer(t, func(conn net.Conn) {
		request := readRequest(t, conn)
		if request["method"] != "getInfo" {
			t.Fatalf("expected getInfo, got %#v", request["method"])
		}
		params, _ := request["params"].(map[string]any)
		if operationID, _ := params["operation_id"].(string); operationID == "" {
			t.Fatalf("expected request operation_id, got %#v", params)
		}
		writeResponse(t, conn, request["id"], map[string]any{"name": "Open Browser Use"})
	})
	defer closeServer()

	client := NewClient(Options{SocketPath: socketPath, Timeout: time.Second})
	defer client.Close()

	result, err := client.GetInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, map[string]any{"name": "Open Browser Use"}) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDispatchesNotifications(t *testing.T) {
	socketPath, closeServer := startSDKServer(t, func(conn net.Conn) {
		request := readRequest(t, conn)
		if err := wire.WriteJSON(conn, map[string]any{
			"jsonrpc": "2.0",
			"method":  "onDownloadChange",
			"params":  map[string]any{"id": "1", "status": "started"},
		}); err != nil {
			t.Error(err)
			return
		}
		writeResponse(t, conn, request["id"], map[string]any{"ok": true})
	})
	defer closeServer()

	client := NewClient(Options{SocketPath: socketPath, Timeout: time.Second})
	defer client.Close()
	notifications := []Notification{}
	client.OnNotification(func(notification Notification) {
		notifications = append(notifications, notification)
	})
	if _, err := client.GetInfo(); err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 || notifications[0].Method != "onDownloadChange" {
		t.Fatalf("unexpected notifications: %#v", notifications)
	}
}

func TestHighLevelBrowserTabHelpers(t *testing.T) {
	socketPath, closeServer := startSDKServer(t, func(conn net.Conn) {
		var calls [][2]string
		for {
			request := readRequest(t, conn)
			if request == nil {
				break
			}
			method, _ := request["method"].(string)
			params, _ := request["params"].(map[string]any)
			cdpMethod, _ := params["method"].(string)
			calls = append(calls, [2]string{method, cdpMethod})
			result := map[string]any{}
			switch {
			case method == "createTab":
				result = map[string]any{"id": 123}
			case method == "executeCdp" && cdpMethod == "Page.navigate":
				result = map[string]any{"frameId": "frame-1"}
			case method == "executeCdp" && cdpMethod == "Runtime.evaluate":
				commandParams, _ := params["commandParams"].(map[string]any)
				expression, _ := commandParams["expression"].(string)
				switch {
				case expression == "({ href: window.location.href, readyState: document.readyState })":
					result = map[string]any{"result": map[string]any{"value": map[string]any{
						"href":       "https://example.test/issues",
						"readyState": "interactive",
					}}}
				case expression == "document.title ?? ''":
					result = map[string]any{"result": map[string]any{"value": "Issues - open-browser-use"}}
				case expression == "location.href":
					result = map[string]any{"result": map[string]any{"value": "https://example.test/issues"}}
				default:
					result = map[string]any{"result": map[string]any{"value": "Open\nClosed\nIssues\nStarred"}}
				}
			}
			writeResponse(t, conn, request["id"], result)
			if len(calls) >= 14 {
				expected := [][2]string{
					{"createTab", ""},
					{"attach", ""},
					{"executeCdp", "Page.enable"},
					{"executeCdp", "Page.navigate"},
					{"executeCdp", "Page.enable"},
					{"executeCdp", "Runtime.evaluate"},
					{"executeCdp", "Page.enable"},
					{"executeCdp", "Runtime.evaluate"},
					{"executeCdp", "Runtime.evaluate"},
					{"executeCdp", "Runtime.evaluate"},
					{"executeCdp", "Runtime.evaluate"},
					{"executeCdp", "Runtime.evaluate"},
					{"executeCdp", "Page.close"},
					{"finalizeTabs", ""},
				}
				if !reflect.DeepEqual(calls, expected) {
					t.Errorf("unexpected calls: %#v", calls)
				}
				return
			}
		}
	})
	defer closeServer()

	browser, err := ConnectOpenBrowserUse(Options{SocketPath: socketPath, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()

	tab, err := browser.NewTab()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tab.Goto("https://example.test/issues", GotoOptions{
		WaitUntil: LoadStateDOMContentLoaded,
		Timeout:   time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tab.WaitForLoadState(WaitForLoadStateOptions{
		State:   LoadStateDOMContentLoaded,
		Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	title, err := tab.Title()
	if err != nil {
		t.Fatal(err)
	}
	if title != "Issues - open-browser-use" {
		t.Fatalf("unexpected title: %q", title)
	}
	url, err := tab.URL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://example.test/issues" {
		t.Fatalf("unexpected URL: %q", url)
	}
	text, err := tab.DOMSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if text != "Open\nClosed\nIssues\nStarred" {
		t.Fatalf("unexpected snapshot: %q", text)
	}
	locatorText, err := tab.Locator("body").InnerText(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if locatorText != "Open\nClosed\nIssues\nStarred" {
		t.Fatalf("unexpected locator text: %q", locatorText)
	}
	if _, err := tab.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Client.FinalizeTabs(nil); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadAndClipboardWrappers(t *testing.T) {
	expected := []struct {
		method string
		params map[string]any
	}{
		{"waitForDownload", map[string]any{"tabId": float64(123), "timeoutMs": float64(5000)}},
		{"downloadPath", map[string]any{"downloadId": "download-1"}},
		{"readClipboardText", map[string]any{"tabId": float64(123)}},
		{"writeClipboardText", map[string]any{"tabId": float64(123), "text": "hello"}},
	}
	socketPath, closeServer := startSDKServer(t, func(conn net.Conn) {
		for _, item := range expected {
			request := readRequest(t, conn)
			if request["method"] != item.method {
				t.Fatalf("expected %s, got %#v", item.method, request["method"])
			}
			params := request["params"].(map[string]any)
			for key, value := range item.params {
				if !reflect.DeepEqual(params[key], value) {
					t.Fatalf("expected %s=%#v, got %#v", key, value, params[key])
				}
			}
			writeResponse(t, conn, request["id"], map[string]any{})
		}
	})
	defer closeServer()

	client := NewClient(Options{SocketPath: socketPath, Timeout: time.Second})
	defer client.Close()

	if _, err := client.WaitForDownload(123, 5000); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DownloadPath("download-1", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadClipboardText(123); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriteClipboardText(123, "hello"); err != nil {
		t.Fatal(err)
	}
}

func startSDKServer(t *testing.T, handler func(net.Conn)) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "obu-go-sdk-")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(dir, "obu.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()
	return socketPath, func() {
		_ = listener.Close()
		<-done
		_ = os.RemoveAll(dir)
	}
}

func readRequest(t *testing.T, conn net.Conn) map[string]any {
	t.Helper()
	var request map[string]any
	if err := wire.ReadJSON(conn, &request); err != nil {
		return nil
	}
	return request
}

func writeResponse(t *testing.T, conn net.Conn, id any, result any) {
	t.Helper()
	if err := wire.WriteJSON(conn, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}); err != nil {
		t.Fatal(err)
	}
}

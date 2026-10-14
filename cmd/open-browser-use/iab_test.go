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

func privateIabFixture(t *testing.T, scope string, authenticated bool) (int, <-chan map[string]any) {
	t.Helper()
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(pair[0]), "iab-test-server")
	conn, err := net.FileConn(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	calls := make(chan map[string]any, 1)
	go func() {
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "method": "iab.bootstrap", "params": iabBootstrap{
			AuthVersion: 1, Transport: "private", InstanceID: "test", ScopeID: scope, GrantID: "grant", Secret: strings.Repeat("A", 43),
		}})
		var auth map[string]any
		if wire.ReadJSON(conn, &auth) != nil {
			return
		}
		wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "id": auth["id"], "result": map[string]any{"authenticated": authenticated, "scopeId": scope}})
		if !authenticated {
			return
		}
		var request map[string]any
		if wire.ReadJSON(conn, &request) != nil {
			return
		}
		calls <- request
		wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "method": "onCDPEvent", "params": map[string]any{}})
		wire.WriteJSON(conn, map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"tabs": 2}})
	}()
	return pair[1], calls
}
func TestIabPrivateChannelAuthAndScope(t *testing.T) {
	fd, calls := privateIabFixture(t, "task", true)
	client, err := connectIab(fd, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.conn.Close()
	if client.bootstrap.Secret != "" {
		t.Fatal("client retained lease secret")
	}
	result, err := client.call("getTabs", map[string]any{}, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if result["result"].(map[string]any)["tabs"] != float64(2) {
		t.Fatal("unexpected result")
	}
	request := <-calls
	params := request["params"].(map[string]any)
	if params["session_id"] != "task" || params["turn_id"] != "turn-1" {
		t.Fatal("scope was not fixed to bootstrap")
	}
	if _, err := client.call("getTabs", map[string]any{"session_id": "other"}, "turn"); err == nil {
		t.Fatal("accepted scope override")
	}
}
func TestIabRequiresAuthenticationAndPrivateBootstrap(t *testing.T) {
	fd, _ := privateIabFixture(t, "task", false)
	if _, err := connectIab(fd, time.Second); err == nil {
		t.Fatal("accepted failed authentication")
	}
	if _, err := connectIab(0, time.Second); err == nil {
		t.Fatal("accepted public stdin as bootstrap")
	}
}
func TestIabCLIUsesInheritedConnectionWithoutSecretOutput(t *testing.T) {
	fd, calls := privateIabFixture(t, "task", true)
	cmd := newRootCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"iab", "tabs", "--channel-fd", strconv.Itoa(fd)})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if (<-calls)["method"] != "getTabs" {
		t.Fatal("wrong IAB method")
	}
	if strings.Contains(out.String(), strings.Repeat("A", 43)) {
		t.Fatal("credential leaked into output")
	}
}

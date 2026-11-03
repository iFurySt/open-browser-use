package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ifuryst/open-browser-use/internal/wire"
)

func domTestSocket(t *testing.T, reply func(map[string]any) map[string]any) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "obu-dom-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "test.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			for {
				var request map[string]any
				err = wire.ReadJSON(conn, &request)
				if err != nil {
					break
				}
				response := reply(request)
				response["id"], response["jsonrpc"] = request["id"], "2.0"
				if err = wire.WriteJSON(conn, response); err != nil {
					break
				}
			}
			conn.Close()
			if err != nil && err != io.EOF {
				t.Errorf("test socket: %v", err)
			}
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done; os.RemoveAll(dir) })
	return path
}

func TestDOMEvaluationSurfacesErrorsAndKeepsLiteralPayload(t *testing.T) {
	expressions := make(chan string, 8)
	path := domTestSocket(t, func(request map[string]any) map[string]any {
		if request["method"] == "attach" {
			return map[string]any{"result": map[string]any{}}
		}
		params := request["params"].(map[string]any)
		command := params["commandParams"].(map[string]any)
		expression := command["expression"].(string)
		expressions <- expression
		if command["awaitPromise"] != true || command["returnByValue"] != true {
			t.Error("evaluation must await and serialize")
		}
		if expression == "throwError" {
			return map[string]any{"result": map[string]any{"exceptionDetails": map[string]any{"text": "fixture error"}}}
		}
		return map[string]any{"result": map[string]any{"result": map[string]any{"value": map[string]any{"ok": true}}}}
	})
	server := newMCPServer(socketOptions{socketPath: path, timeout: time.Second})
	if _, err := server.runDOMTool("evaluate", map[string]any{"tab_id": 42, "expression": "throwError"}); err == nil || !strings.Contains(err.Error(), "fixture error") {
		t.Fatalf("missing page error: %v", err)
	}
	<-expressions
	toolResult, err := server.callTool(json.RawMessage(`{"name":"evaluate","arguments":{"tab_id":42,"expression":"throwError"}}`))
	if err != nil || toolResult["isError"] != true {
		t.Fatalf("MCP hid page failure: %#v, %v", toolResult, err)
	}
	<-expressions
	literal := "Grüße\n'\"` ${x} $(touch nope) </script>"
	if _, err := server.runDOMTool("fill", map[string]any{"tab_id": 42, "fields": []any{map[string]any{"selector": "#message", "text": literal, "expectedValue": ""}}}); err != nil {
		t.Fatal(err)
	}
	expression := <-expressions
	start := strings.LastIndex(expression, ")(")
	var payload map[string]any
	if err := json.Unmarshal([]byte(expression[start+2:len(expression)-1]), &payload); err != nil {
		t.Fatal(err)
	}
	field := payload["fields"].([]any)[0].(map[string]any)
	if field["text"] != literal || field["expectedValue"] != "" {
		t.Fatalf("literal payload changed: %#v", field)
	}
	for _, args := range []map[string]any{
		{"tab_id": 42, "fields": []any{}},
		{"tab_id": 42, "fields": []any{map[string]any{"selector": "#x", "text": "x", "checked": true}}},
		{"tab_id": 42, "fields": []any{map[string]any{"selector": "#x", "text": "x", "typo": true}}},
	} {
		if _, err := server.runDOMTool("fill", args); err == nil {
			t.Errorf("accepted invalid fields: %#v", args)
		}
	}
	if len(expressions) != 0 {
		t.Error("invalid fields reached browser")
	}
}

func TestDOMCLIValidationAndAttachError(t *testing.T) {
	for _, action := range []string{"page-info", "wait-load", "snapshot", "evaluate", "click", "fill", "wait-for", "set-input-files", "capabilities"} {
		command, _, err := newRootCommand().Find([]string{action})
		if err != nil || command.Name() != action {
			t.Fatalf("missing CLI command %s", action)
		}
	}
	for _, args := range [][]string{
		{"snapshot", "--tab-id", "42", "--max-chars", "0"},
		{"fill", "--tab-id", "42", "--fields", `[{"selector":"#x","text":"x"}] {}`},
		{"fill", "--tab-id", "42", "--selector", "#x", "--text", "x", "--checked"},
		{"evaluate", "--tab-id", "42"},
		{"wait-for", "--tab-id", "42"},
	} {
		command := newRootCommand()
		command.SetOut(new(bytes.Buffer))
		command.SetErr(new(bytes.Buffer))
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Errorf("accepted invalid flags: %v", args)
		}
	}
	path := domTestSocket(t, func(request map[string]any) map[string]any {
		if request["method"] != "attach" {
			t.Error("evaluated after failed attach")
		}
		return map[string]any{"error": map[string]any{"message": "tab belongs to another session"}}
	})
	runner := newActionRunner(socketOptions{socketPath: path, timeout: time.Second})
	if _, _, err := runner.evaluateValue(42, "document.title"); err == nil || !strings.Contains(err.Error(), "another session") {
		t.Fatalf("missing attach error: %v", err)
	}
}

func TestDOMFilesLimitsAndEncoding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grüße.txt")
	data := []byte("Grüße\n\x00\xff")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	files, err := readInputFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(files[0]["base64"].(string))
	if err != nil || !bytes.Equal(decoded, data) || files[0]["name"] != "grüße.txt" {
		t.Fatal("file bytes or filename changed")
	}
	for _, paths := range [][]string{nil, {"relative.txt"}, {dir}} {
		if _, err := readInputFiles(paths); err == nil {
			t.Errorf("accepted invalid paths: %v", paths)
		}
	}
	if err := os.WriteFile(path, make([]byte, maxInputFileBytes), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readInputFiles([]string{path}); err != nil {
		t.Fatal(err)
	}
	if _, err := readInputFiles([]string{path, path}); err == nil {
		t.Fatal("aggregate upload limit not enforced")
	}
}

func TestDOMBrowserBootstrapAndCapabilities(t *testing.T) {
	path := domTestSocket(t, func(request map[string]any) map[string]any {
		return map[string]any{"result": map[string]any{"name": "Firefox Backend", "version": "test", "metadata": map[string]any{"extensionInstanceId": "test-host"}}}
	})
	server := newMCPServer(socketOptions{socketPath: path, timeout: time.Second})
	if _, err := server.runDOMTool("select_browser", map[string]any{}); err == nil {
		t.Fatal("empty route accepted")
	}
	response, err := server.runDOMTool("select_browser", map[string]any{"profile": "test-host", "session_id": "task-test"})
	if err != nil {
		t.Fatal(err)
	}
	cap := response.(map[string]any)["result"].(map[string]any)
	if cap["fullCDP"] != false || cap["nativeFileChooser"] != false || cap["fileInputUpload"] != true || server.runner.sessionID != "task-test" {
		t.Fatalf("wrong Zen capabilities: %#v", cap)
	}
	if _, err := server.runDOMTool("select_browser", map[string]any{"profile": "test-host"}); err == nil {
		t.Fatal("pinned route changed")
	}
	unselected := newMCPServer(socketOptions{socketPath: path, timeout: time.Second})
	unselected.runner.browserWorkStarted = true
	if _, err := unselected.runDOMTool("select_browser", map[string]any{"profile": "test-host"}); err == nil {
		t.Fatal("route changed after raw browser work")
	}
}

// Opt in against a real profile. Creates and closes only its own localhost fixture tab.
func TestLiveZenDOM(t *testing.T) {
	profile := os.Getenv("OBU_LIVE_ZEN_PROFILE")
	if profile == "" {
		t.Skip("set OBU_LIVE_ZEN_PROFILE to test an authorized live Zen profile")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>OBU isolated test fixture</title>
<form><label for="message">Message</label><textarea id="message"></textarea><input id="check" type="checkbox">
<select id="choice"><option value="a">A</option><option value="b">B</option></select>
<input id="file" type="file"><input id="disabled" disabled><fieldset disabled><input id="fieldset"></fieldset>
<input id="hidden" style="display:none"><input id="password" type="password" value="fixture-secret">
<button id="submit">Submit</button><button type="button" id="button">Click fixture</button>
</form><pre id="events"></pre><pre id="uploaded"></pre><span id="late" style="display:none">Ready later</span>
<script>document.body.dataset.submits='0';document.body.dataset.clicks='0';
document.querySelector('form').addEventListener('submit',e=>{e.preventDefault();document.body.dataset.submits=String(+document.body.dataset.submits+1)});
document.querySelector('#button').onclick=()=>document.body.dataset.clicks=String(+document.body.dataset.clicks+1);
for(const event of ['input','change'])document.querySelector('form').addEventListener(event,e=>document.querySelector('#events').textContent+=e.target.id+':'+event+'\n');
document.querySelector('#file').onchange=async e=>document.querySelector('#uploaded').textContent=await e.target.files[0].text();
setTimeout(()=>document.querySelector('#late').style.display='inline',400);
</script>`)
	}))
	defer fixture.Close()
	runner := newActionRunner(socketOptions{browser: "zen", profile: profile, sessionID: fmt.Sprintf("obu-dom-smoke-%d", time.Now().UnixNano()), timeout: 10 * time.Second})
	defer func() {
		if _, _, err := runner.runFinalizeTabsAction(nil); err != nil {
			t.Errorf("fixture finalization: %v", err)
		}
	}()
	if _, _, err := runner.invoke("getUserTabs", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.invoke("nameSession", map[string]any{"name": "DOM smoke test - OBU"}); err != nil {
		t.Fatal(err)
	}
	_, tab, err := runner.runOpenTabAction([]string{"--url", fixture.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = runner.waitFor(tab, "#message", "Ready later", fixture.URL, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	op := func(action string, args map[string]any) map[string]any {
		t.Helper()
		response, _, err := runner.domOperation(tab, action, args)
		if err != nil {
			t.Fatal(err)
		}
		return response["result"].(map[string]any)
	}
	eval := func(expression string) any {
		t.Helper()
		response, _, err := runner.evaluateValue(tab, expression)
		if err != nil {
			t.Fatal(err)
		}
		return response["result"]
	}
	literal := "Grüße\n'\"` ${x} $(touch nope)"
	op("fill", map[string]any{"fields": []any{map[string]any{"selector": "#message", "text": literal, "expectedValue": ""}, map[string]any{"selector": "#check", "checked": true}, map[string]any{"selector": "#choice", "text": "b"}}})
	if eval("document.querySelector('#message').value") != literal {
		t.Fatal("literal text changed")
	}
	if eval("document.querySelector('#events').textContent.includes('message:input') && document.querySelector('#events').textContent.includes('message:change')") != true {
		t.Fatal("missing form events")
	}
	for _, badSelector := range []string{"#disabled", "#fieldset", "#hidden", "#password", "input"} {
		if _, _, err := runner.domOperation(tab, "fill", map[string]any{"fields": []any{map[string]any{"selector": "#message", "text": "must stay unchanged"}, map[string]any{"selector": badSelector, "text": "bad"}}}); err == nil {
			t.Fatalf("accepted %s", badSelector)
		}
		if eval("document.querySelector('#message').value") != literal {
			t.Fatal("partially changed fields after failed validation")
		}
	}
	if _, _, err := runner.domOperation(tab, "fill", map[string]any{"fields": []any{map[string]any{"selector": "#message", "text": "overwrite", "expectedValue": ""}}}); err == nil {
		t.Fatal("overwrote unexpected draft")
	}
	snapshot := op("snapshot", map[string]any{"maxChars": 5, "maxElements": 2})
	if snapshot["textTruncated"] != true || snapshot["elementsTruncated"] != true {
		t.Fatal("snapshot bounds ignored")
	}
	full := op("snapshot", map[string]any{"maxChars": 12000, "maxElements": 80})
	encoded, _ := json.Marshal(full)
	if strings.Contains(string(encoded), "fixture-secret") || strings.Contains(string(encoded), `"selector":"#hidden"`) {
		t.Fatal("snapshot exposed hidden fields or password value")
	}
	op("click", map[string]any{"selector": "#button"})
	if eval("document.body.dataset.clicks") != "1" || eval("document.body.dataset.submits") != "0" {
		t.Fatal("unexpected click or submit")
	}
	if eval("Promise.resolve({answer:42})").(map[string]any)["answer"] != float64(42) {
		t.Fatal("promise not awaited")
	}
	if _, _, err := runner.evaluateValue(tab, "(() => {throw new Error('fixture exception')})()"); err == nil {
		t.Fatal("exception hidden")
	}
	filePath := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(filePath, []byte("Datei mit Grüßen"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := readInputFiles([]string{filePath})
	if err != nil {
		t.Fatal(err)
	}
	op("set-input-files", map[string]any{"selector": "#file", "files": files})
	if _, _, err := runner.waitFor(tab, "#uploaded", "Datei mit Grüßen", "", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.domOperation(tab, "set-input-files", map[string]any{"selector": "#file", "files": files}); err == nil {
		t.Fatal("replaced existing file selection")
	}
	op("set-input-files", map[string]any{"selector": "#file", "files": files, "replace": true})
	if eval("document.body.dataset.submits") != "0" {
		t.Fatal("helper submitted form")
	}
	if _, _, err := runner.waitFor(tab, "#never-exists", "", "", 150*time.Millisecond); err == nil {
		t.Fatal("wait did not time out")
	}
}

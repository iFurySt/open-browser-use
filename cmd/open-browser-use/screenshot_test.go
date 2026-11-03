package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func screenshotFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestScreenshotMCPImageAndProtectedOutput(t *testing.T) {
	data := screenshotFixture(t)
	captures := make(chan map[string]any, 10)
	path := domTestSocket(t, func(request map[string]any) map[string]any {
		if request["method"] == "attach" {
			return map[string]any{"result": map[string]any{}}
		}
		params := request["params"].(map[string]any)
		if params["target"].(map[string]any)["tabId"] != float64(42) {
			t.Error("captured wrong tab")
		}
		if params["method"] == "Page.getLayoutMetrics" {
			return map[string]any{"result": map[string]any{"cssContentSize": map[string]any{"x": 0, "y": 0, "width": 4, "height": 3}}}
		}
		captures <- params["commandParams"].(map[string]any)
		return map[string]any{"result": map[string]any{"data": base64.StdEncoding.EncodeToString(data)}}
	})
	server := newMCPServer(socketOptions{socketPath: path, timeout: time.Second})
	server.runner.currentTabID = 42
	result, err := server.callTool(json.RawMessage(`{"name":"screenshot","arguments":{"full_page":true}}`))
	if err != nil || result["isError"] != false {
		t.Fatalf("screenshot MCP failed: %#v %v", result, err)
	}
	content := result["content"].([]map[string]any)
	if len(content) != 2 || content[1]["type"] != "image" || content[1]["mimeType"] != "image/png" || content[1]["data"] != base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("missing MCP image: %#v", content)
	}
	if strings.Contains(content[0]["text"].(string), base64.StdEncoding.EncodeToString(data)) {
		t.Fatal("image duplicated in text")
	}
	clip := (<-captures)["clip"].(map[string]any)
	if clip["width"] != float64(4) || clip["scale"] != float64(1) {
		t.Fatalf("wrong full-page clip: %#v", clip)
	}
	file := filepath.Join(t.TempDir(), "capture.png")
	output, err := server.runScreenshotTool(map[string]any{"output": file})
	if err != nil {
		t.Fatal(err)
	}
	metadata := output.(screenshotResult)
	if metadata.Data != "" || metadata.Width != 4 || metadata.Height != 3 || metadata.Path != file {
		t.Fatalf("wrong metadata: %#v", metadata)
	}
	written, _ := os.ReadFile(file)
	if !bytes.Equal(written, data) {
		t.Fatal("image bytes changed")
	}
	<-captures
	if _, err := server.runScreenshotTool(map[string]any{"output": file}); err == nil {
		t.Fatal("overwrote existing output")
	}
	if len(captures) != 0 {
		t.Fatal("captured before rejecting existing output")
	}
	if _, err := server.runScreenshotTool(map[string]any{"output": file, "overwrite": true}); err != nil {
		t.Fatal(err)
	}
	<-captures
	for _, args := range []map[string]any{
		{"format": "webp"}, {"quality": 80}, {"format": "jpeg", "quality": 101}, {"format": "jpeg", "quality": 1.5},
		{"full_page": true, "clip": map[string]any{"width": 4, "height": 3}},
		{"clip": map[string]any{"x": 0, "y": 0, "width": 4, "height": 3, "scale": 0}},
		{"clip": map[string]any{"x": 0, "y": 0, "width": 10000, "height": 10000}},
		{"output": "relative.png"}, {"overwrite": true}, {"unknown": true}, {"tab_id": 0},
	} {
		if _, err := server.runScreenshotTool(args); err == nil {
			t.Errorf("accepted invalid screenshot arguments: %#v", args)
		}
	}
	if len(captures) != 0 {
		t.Fatal("invalid parameters reached screenshot API")
	}
}

func TestScreenshotCLIAndErrors(t *testing.T) {
	data := screenshotFixture(t)
	path := domTestSocket(t, func(request map[string]any) map[string]any {
		if request["method"] == "attach" {
			return map[string]any{"result": map[string]any{}}
		}
		return map[string]any{"result": map[string]any{"data": base64.StdEncoding.EncodeToString(data)}}
	})
	file := filepath.Join(t.TempDir(), "capture.png")
	command := newRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"screenshot", "--socket", path, "--tab-id", "42", "--output", file, "--clip", `{"x":0,"y":0,"width":4,"height":3}`})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var metadata screenshotResult
	if err := json.Unmarshal(output.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Data != "" || metadata.MimeType != "image/png" {
		t.Fatal("CLI printed image payload")
	}
	for _, response := range []map[string]any{
		{"error": map[string]any{"message": "Missing screenshot permission"}},
		{"result": map[string]any{"data": "not-base64!"}},
		{"result": map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("not-an-image"))}},
	} {
		path := domTestSocket(t, func(request map[string]any) map[string]any {
			if request["method"] == "attach" {
				return map[string]any{"result": map[string]any{}}
			}
			return response
		})
		server := newMCPServer(socketOptions{socketPath: path, timeout: time.Second})
		result, err := server.callTool(json.RawMessage(`{"name":"screenshot","arguments":{"tab_id":42}}`))
		if err != nil || result["isError"] != true {
			t.Fatalf("MCP hid capture failure: %#v %v", result, err)
		}
	}
}

// Run only against an explicitly selected, updated Zen extension/profile.
func TestLiveZenScreenshot(t *testing.T) {
	profile := os.Getenv("OBU_LIVE_ZEN_SCREENSHOT_PROFILE")
	if profile == "" {
		t.Skip("set OBU_LIVE_ZEN_SCREENSHOT_PROFILE to an authorized updated Zen host")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>Zen screenshot fixture</title><style>html,body{margin:0;background:rgb(250,0,0)}body{height:2400px}.bottom{position:absolute;top:2200px;left:0;width:100%;height:200px;background:rgb(0,240,0)}</style><div class="bottom"></div>`)
	}))
	defer fixture.Close()
	runner := newActionRunner(socketOptions{profile: profile, sessionID: fmt.Sprintf("obu-shot-smoke-%d", time.Now().UnixNano()), timeout: 15 * time.Second})
	defer func() {
		if _, _, err := runner.runFinalizeTabsAction(nil); err != nil {
			t.Errorf("finalization: %v", err)
		}
	}()
	if _, _, err := runner.invoke("nameSession", map[string]any{"name": "Screenshot smoke - OBU"}); err != nil {
		t.Fatal(err)
	}
	_, tab, err := runner.runOpenTabAction([]string{"--url", fixture.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.waitFor(tab, ".bottom", "", fixture.URL, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	cap, _, err := runner.capabilities()
	if err != nil || cap["result"].(map[string]any)["cdpScreenshot"] != true {
		t.Fatalf("screenshot support not advertised: %v", err)
	}
	viewport, err := runner.screenshot(tab, screenshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	viewportData, _ := base64.StdEncoding.DecodeString(viewport.Data)
	viewportImage, _, err := image.Decode(bytes.NewReader(viewportData))
	if err != nil {
		t.Fatal(err)
	}
	vr, vg, vb, _ := viewportImage.At(10, 10).RGBA()
	if vr < 60000 || vg > 4000 || vb > 4000 || viewport.Height >= 2400 {
		t.Fatal("viewport screenshot captured the wrong page or dimensions")
	}
	if dir := os.Getenv("OBU_SCREENSHOT_ARTIFACT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "viewport.png"), viewportData, 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(options screenshotOptions, expectedWidth, expectedHeight int) screenshotResult {
		t.Helper()
		result, err := runner.screenshot(tab, options)
		if err != nil {
			t.Fatal(err)
		}
		if result.Width != expectedWidth || result.Height != expectedHeight {
			t.Fatalf("unexpected dimensions %dx%d", result.Width, result.Height)
		}
		data, _ := base64.StdEncoding.DecodeString(result.Data)
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		red, green, blue, _ := img.At(10, 10).RGBA()
		if red < 60000 || green > 4000 || blue > 4000 {
			t.Fatalf("captured wrong tab or empty page: %d,%d,%d", red, green, blue)
		}
		if dir := os.Getenv("OBU_SCREENSHOT_ARTIFACT_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			suffix := "clip.png"
			if options.FullPage {
				suffix = "full-page.png"
			}
			if options.Format == "jpeg" {
				suffix = "clip.jpeg"
			}
			if err := os.WriteFile(filepath.Join(dir, suffix), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	scale := 2.0
	check(screenshotOptions{Clip: &screenshotClip{X: 0, Y: 0, Width: 200, Height: 100, Scale: &scale}}, 400, 200)
	quality := 75
	check(screenshotOptions{Format: "jpeg", Quality: &quality, Clip: &screenshotClip{X: 0, Y: 0, Width: 120, Height: 90}}, 120, 90)
	full, err := runner.screenshot(tab, screenshotOptions{FullPage: true})
	if err != nil {
		t.Fatal(err)
	}
	if full.Height != 2400 {
		t.Fatalf("full-page capture height: %d", full.Height)
	}
	t.Logf("viewport %dx%d, scaled clip 400x200, JPEG 120x90, full page %dx%d", viewport.Width, viewport.Height, full.Width, full.Height)
	data, _ := base64.StdEncoding.DecodeString(full.Data)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	red, green, _, _ := img.At(10, 2300).RGBA()
	if red > 4000 || green < 60000 {
		t.Fatal("full-page screenshot did not include offscreen footer")
	}
	if dir := os.Getenv("OBU_SCREENSHOT_ARTIFACT_DIR"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "full-page.png"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	response, _, err := runner.invoke("executeCdp", map[string]any{"target": map[string]any{"tabId": tab}, "method": "Page.captureScreenshot", "commandParams": map[string]any{"format": "webp"}})
	if err != nil || evaluationResponseError(response) == nil {
		t.Fatal("unsupported format did not return a browser error")
	}
}

func TestScreenshotCapabilityReflectsInstalledExtension(t *testing.T) {
	for _, supported := range []bool{false, true} {
		path := domTestSocket(t, func(request map[string]any) map[string]any {
			info := map[string]any{"name": "Open Browser Use Firefox", "version": "test"}
			if supported {
				info["capabilities"] = map[string]any{"screenshot": map[string]any{"cdp": true}}
			}
			return map[string]any{"result": info}
		})
		response, _, err := newActionRunner(socketOptions{socketPath: path, timeout: time.Second}).capabilities()
		if err != nil || response["result"].(map[string]any)["cdpScreenshot"] != supported {
			t.Fatalf("incorrect installed-extension capability: %v", err)
		}
	}
}

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifuryst/open-browser-use/internal/host"
	"github.com/spf13/cobra"
)

const mcpProtocolVersion = "2025-06-18"

type mcpServer struct {
	options socketOptions
	runner  *actionRunner
}

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newMCPCommand() *cobra.Command {
	options := socketOptions{sessionID: defaultMCPSessionID}
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP stdio server for Open Browser Use",
		Long: `Run an MCP stdio server for Open Browser Use.

The server exposes browser tools over newline-delimited JSON-RPC on stdin/stdout.
It is intended for agent runtimes that can launch local MCP servers, for example
with the command "obu mcp".`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			server := newMCPServer(options)
			return server.serve(os.Stdin, os.Stdout)
		},
	}
	addSocketFlags(cmd, &options)
	return cmd
}

func newMCPServer(options socketOptions) *mcpServer {
	if options.socketDir == "" {
		options.socketDir = host.DefaultSocketDir
	}
	if options.timeout == 0 {
		options.timeout = 10 * time.Second
	}
	if options.sessionID == "" {
		options.sessionID = defaultMCPSessionID
	}
	return &mcpServer{
		options: options,
		runner:  newActionRunner(options),
	}
}

func (server *mcpServer) serve(reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	encoder := json.NewEncoder(writer)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		responses := server.handleLine([]byte(line))
		for _, response := range responses {
			if response == nil {
				continue
			}
			if err := encoder.Encode(response); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func (server *mcpServer) handleLine(payload []byte) []map[string]any {
	if len(payload) > 0 && payload[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(payload, &batch); err != nil {
			return []map[string]any{mcpErrorResponse(nil, -32700, "Parse error")}
		}
		responses := make([]map[string]any, 0, len(batch))
		for _, item := range batch {
			if response := server.handleMessage(item); response != nil {
				responses = append(responses, response)
			}
		}
		return responses
	}
	if response := server.handleMessage(payload); response != nil {
		return []map[string]any{response}
	}
	return nil
}

func (server *mcpServer) handleMessage(payload []byte) map[string]any {
	var request mcpRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return mcpErrorResponse(nil, -32700, "Parse error")
	}
	if request.Method == "" {
		return mcpErrorResponse(request.ID, -32600, "Invalid request")
	}
	if request.ID == nil {
		return nil
	}

	switch request.Method {
	case "initialize":
		return mcpResultResponse(request.ID, server.initializeResult(request.Params))
	case "ping":
		return mcpResultResponse(request.ID, map[string]any{})
	case "tools/list":
		return mcpResultResponse(request.ID, map[string]any{"tools": mcpTools()})
	case "tools/call":
		result, err := server.callTool(request.Params)
		if err != nil {
			return mcpErrorResponse(request.ID, -32602, err.Error())
		}
		return mcpResultResponse(request.ID, result)
	default:
		return mcpErrorResponse(request.ID, -32601, "Method not found")
	}
}

func (server *mcpServer) initializeResult(params json.RawMessage) map[string]any {
	protocolVersion := mcpProtocolVersion
	var value struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &value) == nil {
		if value.ProtocolVersion == "2024-11-05" || value.ProtocolVersion == "2025-03-26" || value.ProtocolVersion == mcpProtocolVersion {
			protocolVersion = value.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "open-browser-use",
			"version": version,
		},
	}
}

func mcpResultResponse(id any, result any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
}

func mcpErrorResponse(id any, code int, message string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": mcpError{
			Code:    code,
			Message: message,
		},
	}
}

func mcpTools() []mcpTool {
	return []mcpTool{
		{
			Name:        "ping",
			Title:       "Ping Browser Backend",
			Description: "Check that the Open Browser Use browser backend is reachable.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        "info",
			Title:       "Get Browser Backend Info",
			Description: "Return Open Browser Use browser backend metadata.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        "tabs",
			Title:       "List Session Tabs",
			Description: "List tabs currently managed by the active Open Browser Use session.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        "user_tabs",
			Title:       "List User Tabs",
			Description: "List visible Chrome tabs from the user's current browser profile.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        "history",
			Title:       "Search Chrome History",
			Description: "Search the user's Chrome history.",
			InputSchema: objectSchema(map[string]any{
				"query": stringSchema("Search query. Leave empty to list recent history."),
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum result count.",
					"minimum":     1,
					"default":     100,
				},
				"from": stringSchema("Optional ISO start date."),
				"to":   stringSchema("Optional ISO end date."),
			}, nil),
		},
		{
			Name:        "open_tab",
			Title:       "Open Browser Tab",
			Description: "Open a new managed browser tab and optionally navigate it to a URL.",
			InputSchema: objectSchema(map[string]any{
				"url": stringSchema("Optional URL to navigate after creating the tab."),
			}, nil),
		},
		{
			Name:        "claim_tab",
			Title:       "Claim Existing Tab",
			Description: "Claim an existing Chrome tab by id for the current browser session.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Chrome tab id to claim."),
			}, []string{"tab_id"}),
		},
		{
			Name:        "claim_status",
			Title:       "Inspect Tab Claim",
			Description: "Inspect whether a Chrome tab is claimable and which session owns it.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Chrome tab id to inspect."),
			}, []string{"tab_id"}),
		},
		{
			Name:        "navigate",
			Title:       "Navigate Tab",
			Description: "Navigate a managed tab. If tab_id is omitted, uses the current tab from a prior open_tab or claim_tab call.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Optional Chrome tab id. Defaults to the current tab."),
				"url":    stringSchema("URL to navigate to."),
			}, []string{"url"}),
		},
		{
			Name:        "wait_load",
			Title:       "Wait For Page Load",
			Description: "Wait for a managed tab to reach a load state.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Optional Chrome tab id. Defaults to the current tab."),
				"state": map[string]any{
					"type":        "string",
					"description": "Load state.",
					"enum":        []string{"load", "domcontentloaded"},
					"default":     "load",
				},
			}, nil),
		},
		{
			Name:        "page_info",
			Title:       "Read Page Info",
			Description: "Read title, URL, readyState, and body text from a managed tab.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Optional Chrome tab id. Defaults to the current tab."),
			}, nil),
		},
		{
			Name: "active_tab", Title: "Get Active Session Tab",
			Description: "Return the logical active tab for this OBU session without activating Chrome.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name: "back", Title: "Navigate Back",
			Description: "Navigate a managed tab one entry back without bringing Chrome to the foreground.",
			InputSchema: objectSchema(map[string]any{"tab_id": integerSchema("Optional managed tab id.")}, nil),
		},
		{
			Name: "forward", Title: "Navigate Forward",
			Description: "Navigate a managed tab one entry forward without bringing Chrome to the foreground.",
			InputSchema: objectSchema(map[string]any{"tab_id": integerSchema("Optional managed tab id.")}, nil),
		},
		{
			Name: "scroll", Title: "Scroll Page",
			Description: "Scroll a managed page by an offset or bring a CSS selector into view.",
			InputSchema: objectSchema(map[string]any{
				"tab_id":   integerSchema("Optional managed tab id."),
				"selector": stringSchema("Optional CSS selector to scroll into view."),
				"delta_x":  integerSchema("Horizontal scroll offset when selector is omitted."),
				"delta_y":  integerSchema("Vertical scroll offset when selector is omitted; defaults to 700."),
			}, nil),
		},
		{
			Name: "extract", Title: "Extract Page Content",
			Description: "Extract bounded text records from elements matching a CSS selector.",
			InputSchema: objectSchema(map[string]any{
				"tab_id":    integerSchema("Optional managed tab id."),
				"selector":  stringSchema("CSS selector; defaults to body."),
				"max_items": integerSchema("Maximum matching elements; defaults to 20."),
				"max_chars": integerSchema("Maximum characters per element; defaults to 4000."),
			}, nil),
		},
		{
			Name: "wait_for", Title: "Wait For Page Condition",
			Description: "Wait for a CSS selector, visible page text, and/or URL fragment.",
			InputSchema: objectSchema(map[string]any{
				"tab_id":       integerSchema("Optional managed tab id."),
				"selector":     stringSchema("Optional CSS selector that must exist."),
				"text":         stringSchema("Optional text that must appear in document body."),
				"url_contains": stringSchema("Optional substring that must appear in the URL."),
			}, nil),
		},
		{
			Name: "click_element", Title: "Click Element",
			Description: "Click the first element matching a CSS selector using page DOM events.",
			InputSchema: objectSchema(map[string]any{"tab_id": integerSchema("Optional managed tab id."), "selector": stringSchema("CSS selector.")}, []string{"selector"}),
		},
		{
			Name: "type_into", Title: "Type Into Element",
			Description: "Set text on the first matching form control and dispatch input/change events.",
			InputSchema: objectSchema(map[string]any{"tab_id": integerSchema("Optional managed tab id."), "selector": stringSchema("CSS selector."), "text": stringSchema("Text to enter.")}, []string{"selector", "text"}),
		},
		{
			Name: "select_option", Title: "Select Option",
			Description: "Select a value on the first matching select control and dispatch events.",
			InputSchema: objectSchema(map[string]any{"tab_id": integerSchema("Optional managed tab id."), "selector": stringSchema("CSS selector."), "value": stringSchema("Option value.")}, []string{"selector", "value"}),
		},
		{
			Name: "screenshot", Title: "Capture Page Screenshot",
			Description: "Capture the visible page surface without activating Chrome.",
			InputSchema: objectSchema(map[string]any{
				"tab_id":  integerSchema("Optional managed tab id."),
				"format":  map[string]any{"type": "string", "enum": []string{"png", "jpeg"}, "default": "png"},
				"quality": integerSchema("JPEG quality from 0 to 100."),
			}, nil),
		},
		{
			Name: "focus_state", Title: "Read Chrome Focus State",
			Description: "Read focused-window and active-tab metadata without changing focus.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name: "close_tab", Title: "Close Managed Tab",
			Description: "Close one tab owned by the current session and return a release receipt.",
			InputSchema: objectSchema(map[string]any{"tab_id": integerSchema("Optional managed tab id.")}, nil),
		},
		{
			Name:        "cdp",
			Title:       "Run CDP Command",
			Description: "Run a Chrome DevTools Protocol command against a managed tab.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Optional Chrome tab id. Defaults to the current tab."),
				"method": stringSchema("Chrome DevTools Protocol method, for example Runtime.evaluate."),
				"params": map[string]any{
					"type":        "object",
					"description": "Chrome DevTools Protocol command parameters.",
					"default":     map[string]any{},
				},
			}, []string{"method"}),
		},
		{
			Name:        "move_mouse",
			Title:       "Move Browser Cursor",
			Description: "Move the browser cursor overlay inside a managed tab.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Optional Chrome tab id. Defaults to the current tab."),
				"x":      numberSchema("Viewport x coordinate."),
				"y":      numberSchema("Viewport y coordinate."),
			}, []string{"x", "y"}),
		},
		{
			Name:        "wait_file_chooser",
			Title:       "Wait For File Chooser",
			Description: "Wait for a managed tab to open a file chooser.",
			InputSchema: objectSchema(map[string]any{
				"tab_id": integerSchema("Optional Chrome tab id. Defaults to the current tab."),
			}, nil),
		},
		{
			Name:        "set_file_chooser_files",
			Title:       "Set File Chooser Files",
			Description: "Set files for an intercepted browser file chooser.",
			InputSchema: objectSchema(map[string]any{
				"file_chooser_id": stringSchema("File chooser id returned by wait_file_chooser."),
				"files": map[string]any{
					"type":        "array",
					"description": "Local file paths to set.",
					"items":       map[string]any{"type": "string"},
					"minItems":    1,
				},
			}, []string{"file_chooser_id", "files"}),
		},
		{
			Name:        "finalize_tabs",
			Title:       "Finalize Tabs",
			Description: "Finalize managed browser tabs. Pass keep entries only for handoff or deliverable tabs.",
			InputSchema: objectSchema(map[string]any{
				"keep": map[string]any{
					"type":        "array",
					"description": "Tabs to keep, for example [{\"tabId\":123,\"status\":\"handoff\"}].",
					"default":     []any{},
					"items":       map[string]any{"type": "object"},
				},
			}, nil),
		},
		{
			Name:        "name_session",
			Title:       "Name Browser Session",
			Description: "Set the Chrome tab group title for the active browser session.",
			InputSchema: objectSchema(map[string]any{
				"name": stringSchema("Session group title, normally '<short task> - OBU'."),
			}, []string{"name"}),
		},
		{
			Name:        "turn_ended",
			Title:       "End Browser Turn",
			Description: "Tell Open Browser Use that the current browser-control turn has ended.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        "reconcile_operation",
			Title:       "Reconcile Browser Operation",
			Description: "Read the durable status or cached result of an operation whose immediate outcome was unknown.",
			InputSchema: objectSchema(map[string]any{
				"operation_id": stringSchema("Operation id reported by a timeout or unknown-outcome error."),
			}, []string{"operation_id"}),
		},
		{
			Name:        "call",
			Title:       "Call Browser RPC",
			Description: "Send an unrestricted Open Browser Use JSON-RPC request to the browser backend.",
			InputSchema: objectSchema(map[string]any{
				"method": stringSchema("Open Browser Use backend method."),
				"params": map[string]any{
					"type":        "object",
					"description": "Backend method parameters.",
					"default":     map[string]any{},
				},
			}, []string{"method"}),
		},
		{
			Name:        "run_action_plan",
			Title:       "Run Action Plan",
			Description: "Run a line-oriented Open Browser Use action plan using the same session and current tab.",
			InputSchema: objectSchema(map[string]any{
				"script": stringSchema("Line-oriented action plan script."),
			}, []string{"script"}),
		},
	}
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func integerSchema(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

func numberSchema(description string) map[string]any {
	return map[string]any{
		"type":        "number",
		"description": description,
	}
}

func (server *mcpServer) callTool(params json.RawMessage) (map[string]any, error) {
	var request struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(params) == 0 {
		return nil, errors.New("tools/call requires params")
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	if request.Name == "" {
		return nil, errors.New("tools/call requires name")
	}
	if request.Arguments == nil {
		request.Arguments = map[string]any{}
	}
	output, err := server.runTool(request.Name, request.Arguments)
	if err != nil {
		return mcpToolErrorResult(err.Error()), nil
	}
	if request.Name == "screenshot" {
		return mcpScreenshotResult(output, optionalStringArg(request.Arguments, "format"))
	}
	return mcpToolResult(output)
}

func (server *mcpServer) runTool(name string, arguments map[string]any) (any, error) {
	switch name {
	case "ping":
		return server.invoke("ping", map[string]any{})
	case "info":
		return server.invoke("getInfo", map[string]any{})
	case "tabs":
		return server.invoke("getTabs", map[string]any{})
	case "user_tabs":
		return server.invoke("getUserTabs", map[string]any{})
	case "history":
		return server.runHistoryTool(arguments)
	case "open_tab":
		return server.runOpenTabTool(arguments)
	case "claim_tab":
		tabID, err := requiredIntArg(arguments, "tab_id")
		if err != nil {
			return nil, err
		}
		response, _, err := server.runner.invoke("claimUserTab", map[string]any{"tabId": tabID})
		if err == nil {
			server.runner.currentTabID = tabID
		}
		return response, err
	case "claim_status":
		tabID, err := requiredIntArg(arguments, "tab_id")
		if err != nil {
			return nil, err
		}
		return server.invoke("claimStatus", map[string]any{"tabId": tabID})
	case "navigate":
		return server.runNavigateTool(arguments)
	case "wait_load":
		return server.runWaitLoadTool(arguments)
	case "page_info":
		return server.runPageInfoTool(arguments)
	case "active_tab":
		response, _, err := server.runner.runActiveTabAction()
		return response, err
	case "back", "forward":
		args, err := optionalTabArgs(arguments)
		if err != nil {
			return nil, err
		}
		delta := -1
		if name == "forward" {
			delta = 1
		}
		response, _, err := server.runner.runHistoryNavigationAction(args, delta)
		return response, err
	case "scroll":
		return server.runScrollTool(arguments)
	case "extract":
		return server.runExtractTool(arguments)
	case "wait_for":
		return server.runWaitForTool(arguments)
	case "click_element", "type_into", "select_option":
		return server.runElementTool(name, arguments)
	case "screenshot":
		return server.runScreenshotTool(arguments)
	case "focus_state":
		return server.invoke("focusState", map[string]any{})
	case "close_tab":
		args, err := optionalTabArgs(arguments)
		if err != nil {
			return nil, err
		}
		response, _, err := server.runner.runCloseTabAction(args)
		return response, err
	case "cdp":
		return server.runCDPTool(arguments)
	case "move_mouse":
		return server.runMoveMouseTool(arguments)
	case "wait_file_chooser":
		return server.runWaitFileChooserTool(arguments)
	case "set_file_chooser_files":
		return server.runSetFileChooserFilesTool(arguments)
	case "finalize_tabs":
		keep, err := optionalArrayArg(arguments, "keep")
		if err != nil {
			return nil, err
		}
		return server.invoke("finalizeTabs", map[string]any{"keep": keep})
	case "name_session":
		name, err := requiredStringArg(arguments, "name")
		if err != nil {
			return nil, err
		}
		return server.invoke("nameSession", map[string]any{"name": name})
	case "turn_ended":
		return server.invoke("turnEnded", map[string]any{})
	case "reconcile_operation":
		operationID, err := requiredStringArg(arguments, "operation_id")
		if err != nil {
			return nil, err
		}
		return server.invoke("reconcileOperation", map[string]any{"operation_id": operationID})
	case "call":
		method, err := requiredStringArg(arguments, "method")
		if err != nil {
			return nil, err
		}
		params, err := optionalObjectArg(arguments, "params")
		if err != nil {
			return nil, err
		}
		return server.invoke(method, params)
	case "run_action_plan":
		script, err := requiredStringArg(arguments, "script")
		if err != nil {
			return nil, err
		}
		return server.runner.run(script)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (server *mcpServer) invoke(method string, params map[string]any) (map[string]any, error) {
	response, _, err := server.runner.invoke(method, params)
	return response, err
}

func (server *mcpServer) runHistoryTool(arguments map[string]any) (any, error) {
	limit := 100
	if value, ok, err := optionalIntArg(arguments, "limit"); err != nil {
		return nil, err
	} else if ok {
		limit = value
	}
	params := map[string]any{
		"query": optionalStringArg(arguments, "query"),
		"limit": limit,
	}
	if from := optionalStringArg(arguments, "from"); from != "" {
		params["from"] = from
	}
	if to := optionalStringArg(arguments, "to"); to != "" {
		params["to"] = to
	}
	return server.invoke("getUserHistory", params)
}

func (server *mcpServer) runOpenTabTool(arguments map[string]any) (any, error) {
	args := []string{}
	if url := optionalStringArg(arguments, "url"); url != "" {
		args = append(args, url)
	}
	response, _, err := server.runner.runOpenTabAction(args)
	return response, err
}

func (server *mcpServer) runNavigateTool(arguments map[string]any) (any, error) {
	url, err := requiredStringArg(arguments, "url")
	if err != nil {
		return nil, err
	}
	args := []string{url}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append([]string{"--tab-id", strconv.Itoa(tabID)}, args...)
	}
	response, _, err := server.runner.runNavigateAction(args)
	return response, err
}

func (server *mcpServer) runWaitLoadTool(arguments map[string]any) (any, error) {
	args := []string{}
	if state := optionalStringArg(arguments, "state"); state != "" {
		args = append(args, "--state", state)
	}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append([]string{"--tab-id", strconv.Itoa(tabID)}, args...)
	}
	response, _, err := server.runner.runWaitLoadAction(args)
	return response, err
}

func (server *mcpServer) runPageInfoTool(arguments map[string]any) (any, error) {
	args := []string{}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append(args, "--tab-id", strconv.Itoa(tabID))
	}
	response, _, err := server.runner.runPageInfoAction(args)
	return response, err
}

func optionalTabArgs(arguments map[string]any) ([]string, error) {
	args := []string{}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append(args, "--tab-id", strconv.Itoa(tabID))
	}
	return args, nil
}

func (server *mcpServer) runScrollTool(arguments map[string]any) (any, error) {
	args, err := optionalTabArgs(arguments)
	if err != nil {
		return nil, err
	}
	if selector := optionalStringArg(arguments, "selector"); selector != "" {
		args = append(args, "--selector", selector)
	}
	for _, field := range []struct{ name, flag string }{{"delta_x", "--delta-x"}, {"delta_y", "--delta-y"}} {
		if value, ok, parseErr := optionalIntArg(arguments, field.name); parseErr != nil {
			return nil, parseErr
		} else if ok {
			args = append(args, field.flag, strconv.Itoa(value))
		}
	}
	response, _, err := server.runner.runScrollAction(args)
	return response, err
}

func (server *mcpServer) runExtractTool(arguments map[string]any) (any, error) {
	args, err := optionalTabArgs(arguments)
	if err != nil {
		return nil, err
	}
	if selector := optionalStringArg(arguments, "selector"); selector != "" {
		args = append(args, "--selector", selector)
	}
	for _, field := range []struct{ name, flag string }{{"max_items", "--max-items"}, {"max_chars", "--max-chars"}} {
		if value, ok, parseErr := optionalIntArg(arguments, field.name); parseErr != nil {
			return nil, parseErr
		} else if ok {
			args = append(args, field.flag, strconv.Itoa(value))
		}
	}
	response, _, err := server.runner.runExtractAction(args)
	return response, err
}

func (server *mcpServer) runWaitForTool(arguments map[string]any) (any, error) {
	args, err := optionalTabArgs(arguments)
	if err != nil {
		return nil, err
	}
	for _, field := range []struct{ name, flag string }{{"selector", "--selector"}, {"text", "--text"}, {"url_contains", "--url-contains"}} {
		if value := optionalStringArg(arguments, field.name); value != "" {
			args = append(args, field.flag, value)
		}
	}
	response, _, err := server.runner.runWaitForAction(args)
	return response, err
}

func (server *mcpServer) runElementTool(name string, arguments map[string]any) (any, error) {
	args, err := optionalTabArgs(arguments)
	if err != nil {
		return nil, err
	}
	selector, err := requiredStringArg(arguments, "selector")
	if err != nil {
		return nil, err
	}
	args = append(args, "--selector", selector)
	var response map[string]any
	switch name {
	case "click_element":
		response, _, err = server.runner.runClickElementAction(args)
	case "type_into":
		text, valueErr := requiredStringValueArg(arguments, "text")
		if valueErr != nil {
			return nil, valueErr
		}
		response, _, err = server.runner.runTypeIntoAction(append(args, "--text", text))
	default:
		value, valueErr := requiredStringValueArg(arguments, "value")
		if valueErr != nil {
			return nil, valueErr
		}
		response, _, err = server.runner.runSelectOptionAction(append(args, "--value", value))
	}
	return response, err
}

func (server *mcpServer) runScreenshotTool(arguments map[string]any) (any, error) {
	args, err := optionalTabArgs(arguments)
	if err != nil {
		return nil, err
	}
	if format := optionalStringArg(arguments, "format"); format != "" {
		args = append(args, "--format", format)
	}
	if quality, ok, parseErr := optionalIntArg(arguments, "quality"); parseErr != nil {
		return nil, parseErr
	} else if ok {
		if quality < 0 || quality > 100 {
			return nil, errors.New("quality must be between 0 and 100")
		}
		args = append(args, "--quality", strconv.Itoa(quality))
	}
	response, _, err := server.runner.runScreenshotAction(args)
	return response, err
}

func (server *mcpServer) runCDPTool(arguments map[string]any) (any, error) {
	method, err := requiredStringArg(arguments, "method")
	if err != nil {
		return nil, err
	}
	params, err := optionalObjectArg(arguments, "params")
	if err != nil {
		return nil, err
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	args := []string{method, string(paramsJSON)}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append([]string{"--tab-id", strconv.Itoa(tabID)}, args...)
	}
	response, _, err := server.runner.runCDPAction(args)
	return response, err
}

func (server *mcpServer) runMoveMouseTool(arguments map[string]any) (any, error) {
	x, err := requiredFloatArg(arguments, "x")
	if err != nil {
		return nil, err
	}
	y, err := requiredFloatArg(arguments, "y")
	if err != nil {
		return nil, err
	}
	args := []string{strconv.FormatFloat(x, 'f', -1, 64), strconv.FormatFloat(y, 'f', -1, 64)}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append([]string{"--tab-id", strconv.Itoa(tabID)}, args...)
	}
	response, _, err := server.runner.runMoveMouseAction(args)
	return response, err
}

func (server *mcpServer) runWaitFileChooserTool(arguments map[string]any) (any, error) {
	args := []string{}
	if tabID, ok, err := optionalIntArg(arguments, "tab_id"); err != nil {
		return nil, err
	} else if ok {
		args = append(args, "--tab-id", strconv.Itoa(tabID))
	}
	response, _, err := server.runner.runWaitFileChooserAction(args)
	return response, err
}

func (server *mcpServer) runSetFileChooserFilesTool(arguments map[string]any) (any, error) {
	fileChooserID, err := requiredStringArg(arguments, "file_chooser_id")
	if err != nil {
		return nil, err
	}
	files, err := requiredStringArrayArg(arguments, "files")
	if err != nil {
		return nil, err
	}
	args := append([]string{fileChooserID}, files...)
	response, _, err := server.runner.runSetFileChooserFilesAction(args)
	return response, err
}

func mcpToolResult(output any) (map[string]any, error) {
	payload, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(payload),
			},
		},
		"structuredContent": output,
		"isError":           false,
	}, nil
}

func mcpScreenshotResult(output any, format string) (map[string]any, error) {
	if format == "" {
		format = "png"
	}
	response, _ := output.(map[string]any)
	result, _ := response["result"].(map[string]any)
	data, _ := result["data"].(string)
	if data == "" {
		return nil, errors.New("screenshot response did not include image data")
	}
	return map[string]any{
		"content": []map[string]any{{
			"type":     "image",
			"data":     data,
			"mimeType": "image/" + format,
		}},
		"structuredContent": map[string]any{
			"format": format,
		},
		"isError": false,
	}, nil
}

func mcpToolErrorResult(message string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": message,
			},
		},
		"isError": true,
	}
}

func requiredStringArg(arguments map[string]any, name string) (string, error) {
	value := optionalStringArg(arguments, name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func requiredStringValueArg(arguments map[string]any, name string) (string, error) {
	value, ok := arguments[name].(string)
	if !ok {
		return "", fmt.Errorf("%s is required and must be a string", name)
	}
	return value, nil
}

func optionalStringArg(arguments map[string]any, name string) string {
	if value, ok := arguments[name].(string); ok {
		return value
	}
	return ""
}

func requiredIntArg(arguments map[string]any, name string) (int, error) {
	value, ok, err := optionalIntArg(arguments, name)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalIntArg(arguments map[string]any, name string) (int, bool, error) {
	raw, ok := arguments[name]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch value := raw.(type) {
	case float64:
		intValue := int(value)
		if float64(intValue) != value {
			return 0, false, fmt.Errorf("%s must be an integer", name)
		}
		return intValue, true, nil
	case int:
		return value, true, nil
	default:
		return 0, false, fmt.Errorf("%s must be an integer", name)
	}
}

func requiredFloatArg(arguments map[string]any, name string) (float64, error) {
	raw, ok := arguments[name]
	if !ok || raw == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	switch value := raw.(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	default:
		return 0, fmt.Errorf("%s must be a number", name)
	}
}

func optionalObjectArg(arguments map[string]any, name string) (map[string]any, error) {
	raw, ok := arguments[name]
	if !ok || raw == nil {
		return map[string]any{}, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return value, nil
}

func optionalArrayArg(arguments map[string]any, name string) ([]any, error) {
	raw, ok := arguments[name]
	if !ok || raw == nil {
		return []any{}, nil
	}
	value, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", name)
	}
	return value, nil
}

func requiredStringArrayArg(arguments map[string]any, name string) ([]string, error) {
	raw, ok := arguments[name]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s is required", name)
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", name)
	}
	out := make([]string, 0, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || value == "" {
			return nil, fmt.Errorf("%s must contain only non-empty strings", name)
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s must contain at least one file", name)
	}
	return out, nil
}

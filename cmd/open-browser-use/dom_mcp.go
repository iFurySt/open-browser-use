package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func domMCPTools() []mcpTool {
	tools := []mcpTool{
		{Name: "select_browser", Description: "Select and pin an explicit browser/profile and optional task session before tab work. Supply browser or profile (an unresolved host can be selected by instance id). Start another server to change a pinned route.", InputSchema: objectSchema(map[string]any{"browser": stringSchema("Browser selector, for example zen."), "profile": stringSchema("Profile directory, display name or extension instance id."), "session_id": stringSchema("Task-unique session id, set before tab work.")}, nil)},
		{Name: "capabilities", Description: "Report backend-derived Chrome/Zen support and limitations, including upload size and evaluation world. Does not prove page-interaction permission is granted.", InputSchema: emptyObjectSchema()},
		{Name: "connected_profiles", Description: "List installed profiles and reachable hosts, including hosts whose profile could not be resolved. Choose a browser/profile at MCP startup or use the selected CLI route.", InputSchema: emptyObjectSchema()},
	}
	field := objectSchema(map[string]any{
		"selector":      stringSchema("Unique visible CSS selector from snapshot."),
		"text":          stringSchema("Literal replacement text or select option value. Empty text is allowed."),
		"checked":       map[string]any{"type": "boolean", "description": "Requested checkbox/radio state."},
		"expectedValue": stringSchema("Only replace if the current value matches this, including an empty string. Preserves unexpected user drafts."),
	}, []string{"selector"})
	field["oneOf"] = []any{map[string]any{"required": []string{"text"}, "not": map[string]any{"required": []string{"checked"}}}, map[string]any{"required": []string{"checked"}, "not": map[string]any{"required": []string{"text"}}}}
	for _, name := range []string{"snapshot", "evaluate", "click", "fill", "wait_for", "set_input_files"} {
		properties := map[string]any{"tab_id": integerSchema("Managed tab id. Defaults to the tab from a prior claim_tab/open_tab call.")}
		var required []string
		description := ""
		switch name {
		case "snapshot":
			description = "Read bounded visible body text and visible elements with CSS selectors. Omits input values and hidden elements; no screenshot or cross-origin iframe inspection."
			properties["max_chars"] = boundedIntegerSchema("Maximum text characters.", 1, 50000, 12000)
			properties["max_elements"] = boundedIntegerSchema("Maximum elements.", 1, 200, 80)
		case "evaluate":
			description = "Evaluate JavaScript and return its serialized value. Propagates page errors; promises are awaited. Zen uses an isolated USER_SCRIPT world, not page JavaScript globals."
			properties["expression"] = stringSchema("JavaScript expression.")
			required = []string{"expression"}
		case "click":
			description = "Click one visible, enabled element selected by CSS. This may submit or otherwise mutate the website; requires authorization for that action."
			properties["selector"] = stringSchema("Unique visible CSS selector.")
			required = []string{"selector"}
		case "fill":
			description = "Replace text/select values and set checkbox states using native setters plus input/change events. Validate every target before changing fields. Does not click submit or press Enter; site event handlers may autosave. Use expectedValue to preserve unexpected drafts."
			properties["fields"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": field}
			required = []string{"fields"}
		case "wait_for":
			description = "Wait until all supplied selector, body text and URL conditions match. At least one condition is required; returns immediately on JavaScript errors."
			properties["selector"] = stringSchema("CSS selector which must become visible.")
			properties["text"] = stringSchema("Visible body text substring.")
			properties["url_includes"] = stringSchema("URL substring.")
			properties["timeout_ms"] = boundedIntegerSchema("Maximum wait milliseconds.", 1, 60000, 10000)
		case "set_input_files":
			description = "Read explicitly authorized local files and assign them to one visible file input via File/DataTransfer events. Works with Zen without a native file chooser. Total bytes <=512 KiB; requires upload authorization. No implicit submit; site change handlers may upload immediately."
			properties["selector"] = stringSchema("Unique visible input[type=file] CSS selector.")
			properties["files"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": stringSchema("Absolute local file path.")}
			properties["replace"] = map[string]any{"type": "boolean", "default": false}
			required = []string{"selector", "files"}
		}
		tools = append(tools, mcpTool{Name: name, Description: description, InputSchema: objectSchema(properties, required)})
	}
	return tools
}

func boundedIntegerSchema(description string, minimum, maximum, value int) map[string]any {
	schema := integerSchema(description)
	schema["minimum"] = minimum
	schema["maximum"] = maximum
	schema["default"] = value
	return schema
}

func boundedIntArg(args map[string]any, name string, minimum, maximum, value int) (int, error) {
	actual, present, err := optionalIntArg(args, name)
	if err != nil {
		return 0, err
	}
	if !present {
		return value, nil
	}
	if actual < minimum || actual > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return actual, nil
}

func (server *mcpServer) runDOMTool(name string, args map[string]any) (any, error) {
	if name == "select_browser" {
		for _, key := range []string{"browser", "profile", "session_id"} {
			if value, ok := args[key]; ok {
				if _, valid := value.(string); !valid {
					return nil, fmt.Errorf("%s must be string", key)
				}
			}
		}
		browser := strings.TrimSpace(optionalStringArg(args, "browser"))
		profile := optionalStringArg(args, "profile")
		session := optionalStringArg(args, "session_id")
		if browser == "" && strings.TrimSpace(profile) == "" {
			return nil, fmt.Errorf("select_browser requires browser or profile")
		}
		if server.runner.browserWorkStarted {
			return nil, fmt.Errorf("select_browser must run before tab work")
		}
		if server.options.browser != "" || server.options.profile != "" {
			return nil, fmt.Errorf("browser/profile is already pinned; start a new MCP server to choose another route")
		}
		options := server.options
		options.browser, options.profile = browser, profile
		if session != "" {
			options.sessionID = session
		}
		runner := newActionRunner(options)
		response, _, err := runner.capabilities()
		if err != nil {
			return nil, err
		}
		server.options, server.runner = options, runner
		return response, nil
	}
	if name == "capabilities" {
		response, _, err := server.runner.capabilities()
		return response, err
	}
	if name == "connected_profiles" {
		installed, err := listInstalledChromeProfiles()
		if err != nil {
			return nil, err
		}
		return map[string]any{"installed": installed, "connected": probeConnectedProfiles(server.options.socketDir)}, nil
	}
	known := false
	for _, n := range []string{"snapshot", "evaluate", "click", "fill", "wait_for", "set_input_files"} {
		if name == n {
			known = true
		}
	}
	if !known {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	tabID, present, err := optionalIntArg(args, "tab_id")
	if err != nil {
		return nil, err
	}
	if !present {
		tabID = server.runner.currentTabID
	}
	if tabID <= 0 {
		return nil, fmt.Errorf("a positive tab_id or a previously claimed/opened tab is required")
	}
	params := map[string]any{}
	switch name {
	case "snapshot":
		chars, err := boundedIntArg(args, "max_chars", 1, 50000, 12000)
		if err != nil {
			return nil, err
		}
		elements, err := boundedIntArg(args, "max_elements", 1, 200, 80)
		if err != nil {
			return nil, err
		}
		params["maxChars"] = chars
		params["maxElements"] = elements
	case "evaluate":
		expression, err := requiredStringArg(args, "expression")
		if err != nil {
			return nil, err
		}
		response, _, err := server.runner.evaluateValue(tabID, expression)
		return response, err
	case "click", "set_input_files":
		selector, err := requiredStringArg(args, "selector")
		if err != nil {
			return nil, err
		}
		params["selector"] = selector
		if name == "set_input_files" {
			raw, err := optionalArrayArg(args, "files")
			if err != nil {
				return nil, err
			}
			var paths []string
			for _, value := range raw {
				path, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("files must be absolute path strings")
				}
				paths = append(paths, path)
			}
			files, err := readInputFiles(paths)
			if err != nil {
				return nil, err
			}
			params["files"] = files
			if value, present := args["replace"]; present {
				replace, ok := value.(bool)
				if !ok {
					return nil, fmt.Errorf("replace must be boolean")
				}
				params["replace"] = replace
			}
		}
	case "fill":
		raw, err := optionalArrayArg(args, "fields")
		if err != nil {
			return nil, err
		}
		payload, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		var fields []formField
		decoder := json.NewDecoder(strings.NewReader(string(payload)))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&fields); err != nil {
			return nil, err
		}
		if err = validateFields(fields); err != nil {
			return nil, err
		}
		params["fields"] = fields
	case "wait_for":
		for _, key := range []string{"selector", "text", "url_includes"} {
			if value, present := args[key]; present {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s must be string", key)
				}
			}
		}
		ms, err := boundedIntArg(args, "timeout_ms", 1, 60000, int(server.options.timeout/time.Millisecond))
		if err != nil {
			return nil, err
		}
		response, _, err := server.runner.waitFor(tabID, optionalStringArg(args, "selector"), optionalStringArg(args, "text"), optionalStringArg(args, "url_includes"), time.Duration(ms)*time.Millisecond)
		return response, err
	}
	response, _, err := server.runner.domOperation(tabID, strings.ReplaceAll(name, "_", "-"), params)
	return response, err
}

package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

//go:embed dom-actions.js
var domActions string

func evaluationResponseError(response map[string]any) error {
	if fault, ok := response["error"]; ok {
		return fmt.Errorf("browser error: %v", fault)
	}
	if result, ok := response["result"].(map[string]any); ok {
		if fault, ok := result["exceptionDetails"]; ok {
			return fmt.Errorf("page JavaScript failed: %v", fault)
		}
	}
	return nil
}

const maxInputFileBytes = 512 * 1024 // Leaves room below browser native-messaging JSON limits.

type formField struct {
	Selector      string  `json:"selector"`
	Text          *string `json:"text,omitempty"`
	Checked       *bool   `json:"checked,omitempty"`
	ExpectedValue *string `json:"expectedValue,omitempty"`
}

func validateFields(fields []formField) error {
	if len(fields) == 0 || len(fields) > 100 {
		return errors.New("fill requires 1 to 100 fields")
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if strings.TrimSpace(field.Selector) == "" || seen[field.Selector] {
			return errors.New("fields require non-empty, distinct selectors")
		}
		seen[field.Selector] = true
		if (field.Text == nil) == (field.Checked == nil) {
			return errors.New("each field requires exactly one of text or checked")
		}
	}
	return nil
}

func (runner *actionRunner) evaluateValue(tabID int, expression string) (map[string]any, int, error) {
	if tabID <= 0 {
		return nil, 0, errors.New("a positive tab id is required; claim or open a tab first")
	}
	if err := runner.attach(tabID); err != nil {
		return nil, tabID, err
	}
	response, _, err := runner.invoke("executeCdp", map[string]any{"target": map[string]any{"tabId": tabID}, "method": "Runtime.evaluate", "commandParams": map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}})
	if err != nil {
		return nil, tabID, err
	}
	if fault, ok := response["error"]; ok {
		return nil, tabID, fmt.Errorf("browser error: %v", fault)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return nil, tabID, errors.New("browser returned an invalid evaluation result")
	}
	if fault, ok := result["exceptionDetails"]; ok {
		return nil, tabID, fmt.Errorf("page JavaScript failed: %v", fault)
	}
	remote, ok := result["result"].(map[string]any)
	if !ok {
		return nil, tabID, errors.New("browser returned no evaluation value")
	}
	if _, present := remote["value"]; !present && remote["type"] != "undefined" {
		return nil, tabID, errors.New("evaluation result is not JSON-serializable; return plain data")
	}
	return map[string]any{"result": remote["value"]}, tabID, nil
}

func (runner *actionRunner) domOperation(tabID int, operation string, params map[string]any) (map[string]any, int, error) {
	params["operation"] = operation
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, tabID, err
	}
	return runner.evaluateValue(tabID, "("+domActions+")("+string(payload)+")")
}

func (runner *actionRunner) capabilities() (map[string]any, int, error) {
	response, tabID, err := runner.invoke("getInfo", map[string]any{})
	if err != nil {
		return nil, tabID, err
	}
	if fault, ok := response["error"]; ok {
		return nil, tabID, fmt.Errorf("browser error: %v", fault)
	}
	info, _ := response["result"].(map[string]any)
	name, _ := info["name"].(string)
	if !strings.Contains(name, "Chrome") && !strings.Contains(name, "Firefox") {
		return nil, tabID, errors.New("unknown browser backend; inspect info before using capabilities")
	}
	firefox := strings.Contains(name, "Firefox")
	screenshot := !firefox
	if caps, ok := info["capabilities"].(map[string]any); ok {
		if support, ok := caps["screenshot"].(map[string]any); ok {
			screenshot = support["cdp"] == true
		}
	}
	return map[string]any{"result": map[string]any{
		"backend": name, "extensionVersion": info["version"], "browserSelector": runner.options.browser, "profileSelector": runner.options.profile,
		"evaluationWorld": map[bool]string{true: "USER_SCRIPT (isolated from page JavaScript globals)", false: "page main world"}[firefox],
		"domInspection":   true, "domInteraction": true, "fileInputUpload": true, "maxUploadBytes": maxInputFileBytes,
		"nativeFileChooser": !firefox, "fullCDP": !firefox, "cdpNetwork": !firefox, "cdpScreenshot": screenshot, "tabGroups": !firefox,
		"pageInteractionRequiresPermission": firefox,
		"limitations":                       map[bool]string{true: "DOM support requires the extension's page-interaction permission. File uploads use synthetic File/DataTransfer events, not a native chooser. Cross-origin frames and trusted input events are not covered.", false: "DOM helpers use CSS selectors in the main document. File uploads use synthetic File/DataTransfer events; use the native chooser for larger files or sites requiring trusted selection."}[firefox],
	}}, tabID, nil
}

func (runner *actionRunner) waitFor(tabID int, selector, text, urlIncludes string, timeout time.Duration) (map[string]any, int, error) {
	if selector == "" && text == "" && urlIncludes == "" {
		return nil, tabID, errors.New("wait-for requires selector, text or url-includes")
	}
	if timeout <= 0 || timeout > 60*time.Second {
		return nil, tabID, errors.New("wait timeout must be positive and at most 60s")
	}
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, tabID, fmt.Errorf("timed out waiting for page conditions in tab %d", tabID)
		}
		attempt := *runner
		if attempt.options.timeout > remaining {
			attempt.options.timeout = remaining
		}
		response, _, err := attempt.domOperation(tabID, "wait-for", map[string]any{"selector": selector, "text": text, "urlIncludes": urlIncludes})
		if err != nil {
			return nil, tabID, err
		}
		result, _ := response["result"].(map[string]any)
		if result["matched"] == true {
			return response, tabID, nil
		}
		time.Sleep(min(100*time.Millisecond, time.Until(deadline)))
	}
}

func readInputFiles(paths []string) ([]map[string]any, error) {
	if len(paths) == 0 || len(paths) > 100 {
		return nil, errors.New("1 to 100 files are required")
	}
	var files []map[string]any
	var total int
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, errors.New("upload paths must be absolute")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			file.Close()
			return nil, errors.New("upload must be a regular file")
		}
		data, err := io.ReadAll(io.LimitReader(file, int64(maxInputFileBytes-total+1)))
		file.Close()
		if err != nil {
			return nil, err
		}
		total += len(data)
		if total > maxInputFileBytes {
			return nil, fmt.Errorf("DOM uploads are limited to %d bytes in total; use the Chrome native chooser or ask the user to select larger files", maxInputFileBytes)
		}
		kind := mime.TypeByExtension(filepath.Ext(path))
		if kind == "" {
			kind = "application/octet-stream"
		}
		files = append(files, map[string]any{"name": filepath.Base(path), "type": kind, "base64": base64.StdEncoding.EncodeToString(data)})
	}
	return files, nil
}

func newDOMCommands() []*cobra.Command {
	var commands []*cobra.Command
	for _, action := range []string{"capabilities", "page-info", "wait-load", "snapshot", "evaluate", "click", "fill", "wait-for", "set-input-files"} {
		action := action
		var options socketOptions
		var tabID, maxChars, maxElements int
		var selector, text, textFile, expression, expressionFile, fieldsJSON, fieldsFile, state, urlIncludes, expected string
		var checked, replace bool
		var paths []string
		command := &cobra.Command{Use: action, Short: "Browser " + action + " using the shared CLI/MCP runner", Args: cobra.NoArgs}
		addSocketFlags(command, &options)
		if action != "capabilities" {
			command.Flags().IntVar(&tabID, "tab-id", 0, "managed tab id")
			command.MarkFlagRequired("tab-id")
		}
		switch action {
		case "snapshot":
			command.Flags().IntVar(&maxChars, "max-chars", 12000, "maximum text characters (1-50000)")
			command.Flags().IntVar(&maxElements, "max-elements", 80, "maximum elements (1-200)")
		case "evaluate":
			command.Flags().StringVar(&expression, "expression", "", "JavaScript expression")
			command.Flags().StringVar(&expressionFile, "expression-file", "", "UTF-8 file containing the expression")
		case "click", "set-input-files":
			command.Flags().StringVar(&selector, "selector", "", "unique visible CSS selector")
			command.MarkFlagRequired("selector")
		case "fill":
			command.Flags().StringVar(&selector, "selector", "", "unique visible CSS selector")
			command.Flags().StringVar(&text, "text", "", "literal text value")
			command.Flags().StringVar(&textFile, "text-file", "", "UTF-8 file containing literal text")
			command.Flags().BoolVar(&checked, "checked", false, "checkbox/radio state")
			command.Flags().StringVar(&expected, "expected-value", "", "only replace this existing text value")
			command.Flags().StringVar(&fieldsJSON, "fields", "", "JSON fields array")
			command.Flags().StringVar(&fieldsFile, "fields-file", "", "UTF-8 JSON fields file")
		case "wait-load":
			command.Flags().StringVar(&state, "state", "load", "load or domcontentloaded")
		case "wait-for":
			command.Flags().StringVar(&selector, "selector", "", "CSS selector which must become visible")
			command.Flags().StringVar(&text, "text", "", "visible body text substring")
			command.Flags().StringVar(&urlIncludes, "url-includes", "", "URL substring")
		}
		if action == "set-input-files" {
			command.Flags().StringArrayVar(&paths, "file", nil, "absolute file path (repeatable)")
			command.Flags().BoolVar(&replace, "replace", false, "replace files already selected")
		}
		command.RunE = func(c *cobra.Command, _ []string) error {
			runner := newActionRunner(options)
			var response map[string]any
			var err error
			switch action {
			case "capabilities":
				response, _, err = runner.capabilities()
			case "page-info":
				response, _, err = runner.runPageInfoAction([]string{"--tab-id", strconv.Itoa(tabID)})
			case "wait-load":
				response, _, err = runner.runWaitLoadAction([]string{"--tab-id", strconv.Itoa(tabID), "--state", state})
			case "snapshot":
				if maxChars < 1 || maxChars > 50000 || maxElements < 1 || maxElements > 200 {
					return errors.New("snapshot limits out of range")
				}
				response, _, err = runner.domOperation(tabID, action, map[string]any{"maxChars": maxChars, "maxElements": maxElements})
			case "evaluate":
				if (expression == "") == (expressionFile == "") {
					return errors.New("provide exactly one of expression or expression-file")
				}
				if expressionFile != "" {
					data, readErr := os.ReadFile(expressionFile)
					if readErr != nil {
						return readErr
					}
					expression = string(data)
				}
				response, _, err = runner.evaluateValue(tabID, expression)
			case "click":
				response, _, err = runner.domOperation(tabID, action, map[string]any{"selector": selector})
			case "fill":
				var fields []formField
				if fieldsJSON != "" || fieldsFile != "" {
					if selector != "" || c.Flags().Changed("text") || textFile != "" || c.Flags().Changed("checked") || c.Flags().Changed("expected-value") || fieldsJSON != "" && fieldsFile != "" {
						return errors.New("use either fields/fields-file or single-field flags")
					}
					if fieldsFile != "" {
						data, readErr := os.ReadFile(fieldsFile)
						if readErr != nil {
							return readErr
						}
						fieldsJSON = string(data)
					}
					decoder := json.NewDecoder(strings.NewReader(fieldsJSON))
					decoder.DisallowUnknownFields()
					if decodeErr := decoder.Decode(&fields); decodeErr != nil {
						return decodeErr
					}
					if decoder.Decode(new(any)) != io.EOF {
						return errors.New("fields must contain one JSON array")
					}
				} else {
					textSet := c.Flags().Changed("text") || textFile != ""
					if textSet == c.Flags().Changed("checked") || c.Flags().Changed("text") && textFile != "" {
						return errors.New("provide one of text, text-file or checked")
					}
					field := formField{Selector: selector}
					if textFile != "" {
						data, readErr := os.ReadFile(textFile)
						if readErr != nil {
							return readErr
						}
						text = string(data)
					}
					if textSet {
						field.Text = &text
					} else {
						field.Checked = &checked
					}
					if c.Flags().Changed("expected-value") {
						field.ExpectedValue = &expected
					}
					fields = []formField{field}
				}
				if err = validateFields(fields); err != nil {
					return err
				}
				response, _, err = runner.domOperation(tabID, action, map[string]any{"fields": fields})
			case "wait-for":
				response, _, err = runner.waitFor(tabID, selector, text, urlIncludes, options.timeout)
			case "set-input-files":
				var files []map[string]any
				files, err = readInputFiles(paths)
				if err == nil {
					response, _, err = runner.domOperation(tabID, action, map[string]any{"selector": selector, "files": files, "replace": replace})
				}
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(c.OutOrStdout()).Encode(response)
		}
		commands = append(commands, command)
	}
	return commands
}

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const maxScreenshotBytes = 32 * 1024 * 1024
const maxScreenshotPixels = 32 * 1024 * 1024

type screenshotClip struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  float64  `json:"width"`
	Height float64  `json:"height"`
	Scale  *float64 `json:"scale,omitempty"`
}

type screenshotOptions struct {
	Format    string          `json:"format"`
	Quality   *int            `json:"quality,omitempty"`
	Clip      *screenshotClip `json:"clip,omitempty"`
	FullPage  bool            `json:"full_page,omitempty"`
	Output    string          `json:"output,omitempty"`
	Overwrite bool            `json:"overwrite,omitempty"`
}

type screenshotResult struct {
	TabID    int    `json:"tabId"`
	MimeType string `json:"mimeType"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
	Path     string `json:"path,omitempty"`
	Data     string `json:"data,omitempty"`
}

func (clip *screenshotClip) validate() error {
	if clip.Scale == nil {
		scale := 1.0
		clip.Scale = &scale
	}
	for _, value := range []float64{clip.X, clip.Y, clip.Width, clip.Height, *clip.Scale} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("clip values must be finite")
		}
	}
	if clip.X < 0 || clip.Y < 0 || clip.Width <= 0 || clip.Height <= 0 || *clip.Scale <= 0 || *clip.Scale > 4 {
		return errors.New("clip requires nonnegative x/y, positive width/height, and scale in (0,4]")
	}
	scale := *clip.Scale
	if clip.Width*scale > 16384 || clip.Height*scale > 16384 || clip.Width*clip.Height*scale*scale > maxScreenshotPixels {
		return errors.New("screenshot exceeds 16384 pixels per side or 32 megapixels")
	}
	return nil
}

func (options *screenshotOptions) validate() error {
	if options.Format == "" {
		options.Format = "png"
	}
	if options.Format != "png" && options.Format != "jpeg" {
		return errors.New("format must be png or jpeg")
	}
	if options.Quality != nil && (options.Format != "jpeg" || *options.Quality < 0 || *options.Quality > 100) {
		return errors.New("quality requires jpeg and an integer from 0 to 100")
	}
	if options.FullPage && options.Clip != nil {
		return errors.New("full_page and clip are mutually exclusive")
	}
	if options.Clip != nil {
		if err := options.Clip.validate(); err != nil {
			return err
		}
	}
	if options.Output != "" {
		if !filepath.IsAbs(options.Output) {
			return errors.New("screenshot output must be an absolute path")
		}
		info, err := os.Stat(options.Output)
		if err == nil && (!options.Overwrite || !info.Mode().IsRegular()) {
			return errors.New("output exists; use overwrite only when replacing a regular file is authorized")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if options.Overwrite && options.Output == "" {
		return errors.New("overwrite requires output")
	}
	return nil
}

func (runner *actionRunner) screenshot(tabID int, options screenshotOptions) (screenshotResult, error) {
	var result screenshotResult
	if tabID <= 0 {
		return result, errors.New("a positive tab id is required")
	}
	if err := options.validate(); err != nil {
		return result, err
	}
	if err := runner.attach(tabID); err != nil {
		return result, err
	}
	invoke := func(method string, params map[string]any) (map[string]any, error) {
		response, _, err := runner.invoke("executeCdp", map[string]any{"target": map[string]any{"tabId": tabID}, "method": method, "commandParams": params})
		if err == nil {
			err = evaluationResponseError(response)
		}
		if err != nil {
			return nil, err
		}
		value, ok := response["result"].(map[string]any)
		if !ok {
			return nil, errors.New("browser returned an invalid screenshot response")
		}
		return value, nil
	}
	if options.FullPage {
		layout, err := invoke("Page.getLayoutMetrics", map[string]any{})
		if err != nil {
			return result, err
		}
		size, ok := layout["cssContentSize"].(map[string]any)
		if !ok {
			return result, errors.New("browser returned no CSS content size")
		}
		payload, _ := json.Marshal(size)
		options.Clip = &screenshotClip{}
		if err := json.Unmarshal(payload, options.Clip); err != nil {
			return result, err
		}
		if err := options.Clip.validate(); err != nil {
			return result, err
		}
	}
	params := map[string]any{"format": options.Format, "fromSurface": true}
	if options.Quality != nil {
		params["quality"] = *options.Quality
	}
	if options.Clip != nil {
		params["clip"] = options.Clip
		params["captureBeyondViewport"] = true
	}
	response, err := invoke("Page.captureScreenshot", params)
	if err != nil {
		return result, err
	}
	encoded, ok := response["data"].(string)
	if !ok || encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(maxScreenshotBytes) {
		return result, errors.New("missing screenshot data or image exceeds 32 MiB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return result, fmt.Errorf("invalid screenshot encoding: %w", err)
	}
	if len(data) > maxScreenshotBytes {
		return result, errors.New("screenshot image exceeds 32 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != options.Format {
		return result, errors.New("browser returned an invalid or unexpected screenshot format")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > maxScreenshotPixels {
		return result, errors.New("screenshot exceeds the pixel limit")
	}
	result = screenshotResult{TabID: tabID, MimeType: "image/" + format, Width: config.Width, Height: config.Height, Bytes: len(data), Data: encoded}
	if options.Output != "" {
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if options.Overwrite {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		file, err := os.OpenFile(options.Output, flags, 0600)
		if err != nil {
			return screenshotResult{}, err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			return screenshotResult{}, writeErr
		}
		if closeErr != nil {
			return screenshotResult{}, closeErr
		}
		result.Path, result.Data = options.Output, ""
	}
	return result, nil
}

func newScreenshotCommand() *cobra.Command {
	var socket socketOptions
	var options screenshotOptions
	var tabID, quality int
	var clipJSON string
	command := &cobra.Command{Use: "screenshot", Short: "Capture a managed tab to a local PNG/JPEG file", Args: cobra.NoArgs}
	addSocketFlags(command, &socket)
	command.Flags().IntVar(&tabID, "tab-id", 0, "managed tab id")
	command.Flags().StringVar(&options.Output, "output", "", "absolute output file path")
	command.Flags().StringVar(&options.Format, "format", "png", "png or jpeg")
	command.Flags().IntVar(&quality, "quality", 92, "JPEG quality, 0-100")
	command.Flags().StringVar(&clipJSON, "clip", "", "page-relative JSON rectangle: x,y,width,height, optional scale")
	command.Flags().BoolVar(&options.FullPage, "full-page", false, "capture the document using CSS layout metrics")
	command.Flags().BoolVar(&options.Overwrite, "overwrite", false, "replace an explicitly authorized existing output")
	command.MarkFlagRequired("tab-id")
	command.MarkFlagRequired("output")
	command.RunE = func(c *cobra.Command, _ []string) error {
		if c.Flags().Changed("quality") {
			options.Quality = &quality
		}
		if clipJSON != "" {
			options.Clip = &screenshotClip{}
			decoder := json.NewDecoder(strings.NewReader(clipJSON))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(options.Clip); err != nil {
				return err
			}
			if decoder.Decode(new(any)) != io.EOF {
				return errors.New("clip must be one JSON object")
			}
		}
		result, err := newActionRunner(socket).screenshot(tabID, options)
		if err != nil {
			return err
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(result)
	}
	return command
}

func screenshotMCPTool() mcpTool {
	clip := objectSchema(map[string]any{
		"x": map[string]any{"type": "number", "minimum": 0}, "y": map[string]any{"type": "number", "minimum": 0},
		"width": map[string]any{"type": "number", "exclusiveMinimum": 0}, "height": map[string]any{"type": "number", "exclusiveMinimum": 0},
		"scale": map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": 4, "default": 1},
	}, []string{"x", "y", "width", "height"})
	return mcpTool{Name: "screenshot", Description: "Capture a managed Chrome/Zen tab as PNG/JPEG. Returns MCP image content unless an absolute output path is supplied. Preserves existing files unless overwrite is explicitly requested. Optional page-relative clip or full_page; maximum 32 megapixels/32 MiB. Requires the updated Zen screenshot extension; full-page Zen capture also needs page-interaction permission.", InputSchema: objectSchema(map[string]any{
		"tab_id":  integerSchema("Managed tab id, defaults to the current claimed/opened tab."),
		"format":  map[string]any{"type": "string", "enum": []string{"png", "jpeg"}, "default": "png"},
		"quality": boundedIntegerSchema("JPEG only.", 0, 100, 92), "clip": clip,
		"full_page": map[string]any{"type": "boolean", "default": false}, "output": stringSchema("Optional absolute local output path."),
		"overwrite": map[string]any{"type": "boolean", "default": false},
	}, nil)}
}

func (server *mcpServer) runScreenshotTool(args map[string]any) (any, error) {
	tabID, present, err := optionalIntArg(args, "tab_id")
	if err != nil {
		return nil, err
	}
	if !present {
		tabID = server.runner.currentTabID
	}
	optionsArgs := map[string]any{}
	for key, value := range args {
		if key != "tab_id" {
			optionsArgs[key] = value
		}
	}
	data, err := json.Marshal(optionsArgs)
	if err != nil {
		return nil, err
	}
	var options screenshotOptions
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return nil, err
	}
	return server.runner.screenshot(tabID, options)
}

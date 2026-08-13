package obu

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/ifuryst/open-browser-use/internal/host"
	"github.com/ifuryst/open-browser-use/internal/wire"
)

const (
	DefaultSessionID          = "open-browser-use-go"
	DefaultTimeout            = 10 * time.Second
	DefaultNavigationTimeout  = 10 * time.Second
	LoadStateDOMContentLoaded = "domcontentloaded"
	LoadStateLoad             = "load"
)

type Params map[string]any

type Options struct {
	SocketPath string
	SocketDir  string
	SessionID  string
	TurnID     string
	Timeout    time.Duration
}

type Notification struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type NotificationHandler func(Notification)

type Client struct {
	options Options

	mu                   sync.Mutex
	conn                 net.Conn
	nextID               int64
	notificationHandlers []NotificationHandler
}

func NewClient(options Options) *Client {
	if options.SessionID == "" {
		options.SessionID = DefaultSessionID
	}
	if options.TurnID == "" {
		options.TurnID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}
	if options.Timeout == 0 {
		options.Timeout = DefaultTimeout
	}
	return &Client{options: options}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) OnNotification(handler NotificationHandler) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notificationHandlers = append(c.notificationHandlers, handler)
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		for i, candidate := range c.notificationHandlers {
			if reflect.ValueOf(candidate).Pointer() == reflect.ValueOf(handler).Pointer() {
				c.notificationHandlers = append(c.notificationHandlers[:i], c.notificationHandlers[i+1:]...)
				return
			}
		}
	}
}

func (c *Client) Request(method string, params Params) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connectLocked(); err != nil {
		return nil, err
	}
	c.nextID++
	requestID := c.nextID
	operationID := ""
	if provided, ok := params["operation_id"].(string); ok {
		operationID = provided
	}
	if operationID == "" {
		operationID = fmt.Sprintf("op-go-%d-%d", time.Now().UnixNano(), requestID)
	}
	merged := Params{
		"session_id":   c.options.SessionID,
		"turn_id":      c.options.TurnID,
		"operation_id": operationID,
	}
	for key, value := range params {
		merged[key] = value
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  merged,
	}
	if err := c.conn.SetDeadline(time.Now().Add(c.options.Timeout)); err != nil {
		return nil, err
	}
	if err := wire.WriteJSON(c.conn, request); err != nil {
		return nil, err
	}
	for {
		var response rpcMessage
		if err := wire.ReadJSON(c.conn, &response); err != nil {
			_ = c.conn.Close()
			c.conn = nil
			if netError, ok := err.(net.Error); ok && netError.Timeout() {
				return nil, fmt.Errorf("%w (operation_id=%s, outcome=unknown; call ReconcileOperation)", err, operationID)
			}
			return nil, err
		}
		if response.ID != nil && idMatches(response.ID, requestID) {
			if response.Error != nil {
				if response.Error.Message != "" {
					return nil, errors.New(response.Error.Message)
				}
				return nil, errors.New("Open Browser Use request failed")
			}
			return response.Result, nil
		}
		if response.ID == nil && response.Method != "" {
			c.dispatchNotificationLocked(Notification{
				Method: response.Method,
				Params: response.Params,
			})
			continue
		}
		// Ignore stale responses and continue until this request's exact id.
	}
}

func (c *Client) GetInfo() (any, error) {
	return c.Request("getInfo", nil)
}

func (c *Client) CreateTab() (any, error) {
	return c.Request("createTab", nil)
}

func (c *Client) GetTabs() (any, error) {
	return c.Request("getTabs", nil)
}

func (c *Client) GetUserTabs() (any, error) {
	return c.Request("getUserTabs", nil)
}

func (c *Client) GetUserHistory(params Params) (any, error) {
	return c.Request("getUserHistory", params)
}

func (c *Client) ClaimUserTab(tabID int) (any, error) {
	return c.Request("claimUserTab", Params{"tabId": tabID})
}

func (c *Client) ClaimStatus(tabID int) (any, error) {
	return c.Request("claimStatus", Params{"tabId": tabID})
}

func (c *Client) CloseTab(tabID int) (any, error) {
	return c.Request("closeTab", Params{"tabId": tabID})
}

func (c *Client) FocusState() (any, error) {
	return c.Request("focusState", nil)
}

func (c *Client) ReconcileOperation(operationID string) (any, error) {
	return c.Request("reconcileOperation", Params{"operation_id": operationID})
}

func (c *Client) FinalizeTabs(keep []Params) (any, error) {
	return c.Request("finalizeTabs", Params{"keep": keep})
}

func (c *Client) NameSession(name string) (any, error) {
	return c.Request("nameSession", Params{"name": name})
}

func (c *Client) Attach(tabID int) (any, error) {
	return c.Request("attach", Params{"tabId": tabID})
}

func (c *Client) Detach(tabID int) (any, error) {
	return c.Request("detach", Params{"tabId": tabID})
}

func (c *Client) ExecuteCDP(tabID int, method string, commandParams Params) (any, error) {
	return c.Request("executeCdp", Params{
		"target":        Params{"tabId": tabID},
		"method":        method,
		"commandParams": commandParams,
	})
}

func (c *Client) MoveMouse(tabID int, x float64, y float64, waitForArrival bool) (any, error) {
	return c.Request("moveMouse", Params{
		"tabId":          tabID,
		"x":              x,
		"y":              y,
		"waitForArrival": waitForArrival,
	})
}

func (c *Client) WaitForFileChooser(tabID int, timeoutMs int) (any, error) {
	params := Params{"tabId": tabID}
	if timeoutMs > 0 {
		params["timeoutMs"] = timeoutMs
	}
	return c.Request("waitForFileChooser", params)
}

func (c *Client) SetFileChooserFiles(fileChooserID string, files []string) (any, error) {
	return c.Request("setFileChooserFiles", Params{"fileChooserId": fileChooserID, "files": files})
}

func (c *Client) WaitForDownload(tabID int, timeoutMs int) (any, error) {
	params := Params{"tabId": tabID}
	if timeoutMs > 0 {
		params["timeoutMs"] = timeoutMs
	}
	return c.Request("waitForDownload", params)
}

func (c *Client) DownloadPath(downloadID string, timeoutMs int) (any, error) {
	params := Params{"downloadId": downloadID}
	if timeoutMs > 0 {
		params["timeoutMs"] = timeoutMs
	}
	return c.Request("downloadPath", params)
}

func (c *Client) BrowserUserHistory(params Params) (any, error) {
	return c.GetUserHistory(params)
}

func (c *Client) ReadClipboardText(tabID int) (any, error) {
	return c.Request("readClipboardText", Params{"tabId": tabID})
}

func (c *Client) WriteClipboardText(tabID int, text string) (any, error) {
	return c.Request("writeClipboardText", Params{"tabId": tabID, "text": text})
}

func (c *Client) ReadClipboard(tabID int) (any, error) {
	return c.Request("readClipboard", Params{"tabId": tabID})
}

func (c *Client) WriteClipboard(tabID int, items []Params) (any, error) {
	return c.Request("writeClipboard", Params{"tabId": tabID, "items": items})
}

func (c *Client) TurnEnded() (any, error) {
	return c.Request("turnEnded", nil)
}

func ActiveSocketPath(socketDir string) (string, error) {
	record, err := host.ReadActiveSocketRecord(socketDir)
	if err != nil {
		return "", err
	}
	return record.SocketPath, nil
}

func ConnectOpenBrowserUse(options Options) (*Browser, error) {
	browser := NewBrowser(NewClient(options))
	if err := browser.Connect(); err != nil {
		return nil, err
	}
	return browser, nil
}

func ConnectActive(options Options) (*Browser, error) {
	if options.SocketPath == "" {
		socketPath, err := ActiveSocketPath(options.SocketDir)
		if err != nil {
			return nil, err
		}
		options.SocketPath = socketPath
	}
	return ConnectOpenBrowserUse(options)
}

func EncodeFrame(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	frame := make([]byte, wire.HeaderBytes+len(payload))
	wire.NativeEndian().PutUint32(frame[:wire.HeaderBytes], uint32(len(payload)))
	copy(frame[wire.HeaderBytes:], payload)
	return frame, nil
}

func (c *Client) connectLocked() error {
	if c.conn != nil {
		return nil
	}
	socketPath := c.options.SocketPath
	if socketPath == "" {
		record, err := host.ReadActiveSocketRecord(c.options.SocketDir)
		if err != nil {
			return err
		}
		socketPath = record.SocketPath
	}
	conn, err := net.DialTimeout("unix", socketPath, c.options.Timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) dispatchNotificationLocked(notification Notification) {
	handlers := append([]NotificationHandler(nil), c.notificationHandlers...)
	for _, handler := range handlers {
		handler(notification)
	}
}

type rpcMessage struct {
	ID     any       `json:"id,omitempty"`
	Method string    `json:"method,omitempty"`
	Params any       `json:"params,omitempty"`
	Result any       `json:"result,omitempty"`
	Error  *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Message string `json:"message,omitempty"`
}

func idMatches(id any, expected int64) bool {
	switch value := id.(type) {
	case float64:
		return int64(value) == expected
	case string:
		return value == fmt.Sprintf("%d", expected)
	default:
		return false
	}
}

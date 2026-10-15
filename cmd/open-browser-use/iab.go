package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/ifuryst/open-browser-use/internal/wire"
	"github.com/spf13/cobra"
)

type iabIdentity struct {
	PID       int    `json:"pid"`
	UID       uint32 `json:"uid"`
	StartTime string `json:"startTime"`
}
type iabBootstrap struct {
	AuthVersion    int         `json:"authVersion"`
	Transport      string      `json:"transport"`
	InstanceID     string      `json:"instanceId"`
	ScopeID        string      `json:"scopeId"`
	GrantID        string      `json:"grantId"`
	Secret         string      `json:"secret"`
	Endpoint       string      `json:"endpoint"`
	ServerIdentity iabIdentity `json:"serverIdentity"`
}
type iabRPC struct {
	conn      net.Conn
	bootstrap iabBootstrap
	sequence  int
	timeout   time.Duration
}

func connectIab(fd int, timeout time.Duration) (*iabRPC, error) {
	if fd < 3 {
		return nil, errors.New("IAB requires a host-provided private descriptor >= 3")
	}
	file := os.NewFile(uintptr(fd), "iab-private-channel")
	if file == nil {
		return nil, errors.New("missing IAB private channel")
	}
	channel, err := net.FileConn(file)
	_ = file.Close()
	if err != nil {
		return nil, errors.New("IAB requires a host-provided private channel; launch this command through open-browser-use-electron")
	}
	_ = channel.SetDeadline(time.Now().Add(timeout))
	var notification struct {
		Method string       `json:"method"`
		Params iabBootstrap `json:"params"`
	}
	payload, err := wire.ReadFrame(channel, 4096)
	if err != nil {
		channel.Close()
		return nil, errors.New("IAB private bootstrap unavailable")
	}
	if err = json.Unmarshal(payload, &notification); err != nil || notification.Method != "iab.bootstrap" ||
		notification.Params.AuthVersion != 1 || notification.Params.InstanceID == "" || notification.Params.ScopeID == "" ||
		notification.Params.GrantID == "" || len(notification.Params.Secret) != 43 {
		channel.Close()
		return nil, errors.New("invalid IAB private bootstrap")
	}
	bootstrap := notification.Params
	conn := channel
	if bootstrap.Transport == "socket" {
		channel.Close()
		if bootstrap.Endpoint == "" || bootstrap.ServerIdentity.PID <= 0 || bootstrap.ServerIdentity.StartTime == "" {
			return nil, errors.New("invalid IAB socket bootstrap")
		}
		conn, err = net.DialTimeout("unix", bootstrap.Endpoint, timeout)
		if err != nil {
			return nil, errors.New("IAB socket unavailable")
		}
		if err = verifyIabPeer(conn, bootstrap.ServerIdentity); err != nil {
			conn.Close()
			return nil, err
		}
	} else if bootstrap.Transport != "private" {
		channel.Close()
		return nil, errors.New("unsupported IAB transport")
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	client := &iabRPC{conn: conn, bootstrap: bootstrap, timeout: timeout}
	response, err := client.raw("auth.connect", map[string]any{
		"authVersion": 1, "instanceId": bootstrap.InstanceID, "grantId": bootstrap.GrantID, "secret": bootstrap.Secret,
	})
	client.bootstrap.Secret = ""
	if err != nil {
		conn.Close()
		return nil, errors.New("IAB authentication failed")
	}
	result, ok := response["result"].(map[string]any)
	if !ok || result["authenticated"] != true || result["scopeId"] != bootstrap.ScopeID {
		conn.Close()
		return nil, errors.New("IAB authentication failed")
	}
	return client, nil
}

func (c *iabRPC) raw(method string, params map[string]any) (map[string]any, error) {
	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}
	c.sequence++
	id := fmt.Sprintf("iab-%d", c.sequence)
	if err := wire.WriteJSON(c.conn, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		payload, err := wire.ReadFrame(c.conn, 8*1024*1024)
		if err != nil {
			return nil, err
		}
		var response map[string]any
		if err = json.Unmarshal(payload, &response); err != nil {
			return nil, err
		}
		if response["id"] != id {
			continue
		}
		if response["error"] != nil {
			return nil, errors.New("IAB request rejected")
		}
		return response, nil
	}
}
func (c *iabRPC) call(method string, params map[string]any, turn string) (map[string]any, error) {
	if supplied, ok := params["session_id"]; ok && supplied != c.bootstrap.ScopeID {
		return nil, errors.New("IAB scope mismatch")
	}
	params["session_id"] = c.bootstrap.ScopeID
	params["turn_id"] = turn
	return c.raw(method, params)
}

func newIabCommand() *cobra.Command {
	var fd int
	var timeout time.Duration
	var turn string
	root := &cobra.Command{Use: "iab", Short: "Control an Electron IAB through host-authorized private or socket transport"}
	root.PersistentFlags().IntVar(&fd, "channel-fd", 3, "inherited private bootstrap channel (never a credential file)")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 15*time.Second, "connection and request timeout")
	root.PersistentFlags().StringVar(&turn, "turn-id", "iab-cli", "bookkeeping turn id; authorization comes from the host")
	root.AddCommand(newIabMCPCommand(&fd, &timeout, &turn))
	add := func(use, description string, args cobra.PositionalArgs, action func(*iabRPC, []string) (map[string]any, error)) {
		root.AddCommand(&cobra.Command{Use: use, Short: description, Args: args, RunE: func(cmd *cobra.Command, values []string) error {
			if timeout <= 0 || timeout > 24*time.Hour {
				return errors.New("invalid IAB timeout")
			}
			client, err := connectIab(fd, timeout)
			if err != nil {
				return err
			}
			defer client.conn.Close()
			response, err := action(client, values)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(response)
		}})
	}
	for _, item := range []struct{ name, method, description string }{
		{"info", "getInfo", "Read authorized IAB capabilities"}, {"tabs", "getTabs", "List every tab in the authorized session"},
		{"ping", "ping", "Ping the authorized IAB"}, {"turn-ended", "turnEnded", "Clean up this turn's temporary tabs"},
	} {
		method := item.method
		add(item.name, item.description, cobra.NoArgs, func(c *iabRPC, _ []string) (map[string]any, error) { return c.call(method, map[string]any{}, turn) })
	}
	cdp := func(c *iabRPC, id int, method string, params map[string]any) (map[string]any, error) {
		if _, err := c.call("attach", map[string]any{"tabId": id}, turn); err != nil {
			return nil, err
		}
		return c.call("executeCdp", map[string]any{"target": map[string]any{"tabId": id}, "method": method, "commandParams": params}, turn)
	}
	add("open-tab [url]", "Create an independent tab", cobra.MaximumNArgs(1), func(c *iabRPC, args []string) (map[string]any, error) {
		params := map[string]any{}
		if len(args) == 1 {
			params["url"] = args[0]
		}
		return c.call("createTab", params, turn)
	})
	add("navigate <tab-id> <url>", "Navigate an explicit tab", cobra.ExactArgs(2), func(c *iabRPC, args []string) (map[string]any, error) {
		id, err := strconv.Atoi(args[0])
		if err != nil || id <= 0 {
			return nil, errors.New("invalid tab id")
		}
		return cdp(c, id, "Page.navigate", map[string]any{"url": args[1]})
	})
	add("close-tab <tab-id>", "Close only this tab", cobra.ExactArgs(1), func(c *iabRPC, args []string) (map[string]any, error) {
		id, err := strconv.Atoi(args[0])
		if err != nil || id <= 0 {
			return nil, errors.New("invalid tab id")
		}
		return cdp(c, id, "Page.close", map[string]any{})
	})
	add("cdp <tab-id> <method> [params-json]", "Run a permitted page-scoped CDP method", cobra.RangeArgs(2, 3), func(c *iabRPC, args []string) (map[string]any, error) {
		id, err := strconv.Atoi(args[0])
		if err != nil || id <= 0 {
			return nil, errors.New("invalid tab id")
		}
		params := map[string]any{}
		if len(args) == 3 {
			if err := json.Unmarshal([]byte(args[2]), &params); err != nil || params == nil {
				return nil, errors.New("invalid CDP params")
			}
		}
		return cdp(c, id, args[1], params)
	})
	add("call <method> [params-json]", "Call an IAB method within the host grant", cobra.RangeArgs(1, 2), func(c *iabRPC, args []string) (map[string]any, error) {
		params := map[string]any{}
		if len(args) == 2 {
			if err := json.Unmarshal([]byte(args[1]), &params); err != nil || params == nil {
				return nil, errors.New("invalid params")
			}
		}
		return c.call(args[0], params, turn)
	})
	return root
}

package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification from the server.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NotificationHandler is called when a notification is received from the server.
type NotificationHandler func(method string, params json.RawMessage)

// ResponseError represents a JSON-RPC 2.0 error.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Conn handles JSON-RPC communication over stdio.
type Conn struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex
	nextID atomic.Int64

	// pending tracks in-flight requests awaiting responses
	pending   map[int64]chan *Response
	pendingMu sync.Mutex

	// notifyHandler handles incoming notifications from the server
	notifyHandler NotificationHandler
	notifyMu      sync.RWMutex

	closed atomic.Bool
}

// NewConn creates a new JSON-RPC connection.
func NewConn(r io.Reader, w io.Writer) *Conn {
	c := &Conn{
		reader:  bufio.NewReader(r),
		writer:  w,
		pending: make(map[int64]chan *Response),
	}
	return c
}

// SetNotificationHandler sets the handler for incoming server notifications.
func (c *Conn) SetNotificationHandler(handler NotificationHandler) {
	c.notifyMu.Lock()
	c.notifyHandler = handler
	c.notifyMu.Unlock()
}

// Call sends a request and waits for the response.
func (c *Conn) Call(method string, params any, result any) error {
	if c.closed.Load() {
		return fmt.Errorf("connection closed")
	}

	id := c.nextID.Add(1)

	// Create response channel
	respCh := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// Send request
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.send(req); err != nil {
		return fmt.Errorf("sending request: %w", err)
	}

	// Wait for response
	resp := <-respCh
	if resp == nil {
		return fmt.Errorf("connection closed while waiting for response")
	}

	if resp.Error != nil {
		return resp.Error
	}

	if result != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("unmarshaling result: %w", err)
		}
	}

	return nil
}

// sendResponse sends a response to a server-initiated request.
func (c *Conn) sendResponse(id int64, result any, err *ResponseError) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
	}
	if err != nil {
		resp.Error = err
	} else if result != nil {
		data, e := json.Marshal(result)
		if e != nil {
			return fmt.Errorf("marshaling result: %w", e)
		}
		resp.Result = data
	}
	return c.send(resp)
}

// Notify sends a notification (no response expected).
func (c *Conn) Notify(method string, params any) error {
	if c.closed.Load() {
		return fmt.Errorf("connection closed")
	}

	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	return c.send(req)
}

// ReadLoop reads messages and dispatches them to waiting callers or notification handlers.
// This should be called in a goroutine.
func (c *Conn) ReadLoop() error {
	for {
		if c.closed.Load() {
			return nil
		}

		body, err := c.readMessageBody()
		if err != nil {
			if c.closed.Load() {
				return nil
			}
			return fmt.Errorf("reading message: %w", err)
		}

		// Peek at the message to determine type
		var peek struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &peek); err != nil {
			continue // Skip malformed messages
		}

		// Server-to-client request (has method AND id)
		if peek.Method != "" && peek.ID != nil {
			// Respond to server requests we care about
			if peek.Method == "window/workDoneProgress/create" {
				// Acknowledge progress token creation
				c.sendResponse(*peek.ID, nil, nil)
			}
			// Dispatch to notification handler too (for logging/tracking)
			c.notifyMu.RLock()
			handler := c.notifyHandler
			c.notifyMu.RUnlock()
			if handler != nil {
				var req Request
				if err := json.Unmarshal(body, &req); err == nil {
					if params, err := json.Marshal(req.Params); err == nil {
						handler(peek.Method, params)
					}
				}
			}
			continue
		}

		// Notification from server (has method, no id)
		if peek.Method != "" && peek.ID == nil {
			var notif Notification
			if err := json.Unmarshal(body, &notif); err != nil {
				continue
			}
			c.notifyMu.RLock()
			handler := c.notifyHandler
			c.notifyMu.RUnlock()
			if handler != nil {
				handler(notif.Method, notif.Params)
			}
			continue
		}

		// Response to our request (has id, no method)
		var resp Response
		if err := json.Unmarshal(body, &resp); err != nil {
			continue
		}

		// Dispatch to waiting caller
		c.pendingMu.Lock()
		ch, ok := c.pending[resp.ID]
		c.pendingMu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

// Close marks the connection as closed.
func (c *Conn) Close() {
	c.closed.Store(true)

	// Close all pending channels
	c.pendingMu.Lock()
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[int64]chan *Response)
	c.pendingMu.Unlock()
}

// send writes a message with LSP headers.
func (c *Conn) send(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(c.writer, header); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := c.writer.Write(data); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}

	return nil
}

// readMessageBody reads a single LSP message and returns the raw body.
func (c *Conn) readMessageBody() ([]byte, error) {
	// Read headers
	var contentLength int
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("reading header line: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}

		if strings.HasPrefix(line, "Content-Length:") {
			lenStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lenStr)
			if err != nil {
				return nil, fmt.Errorf("parsing content length: %w", err)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	// Read body
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	return body, nil
}

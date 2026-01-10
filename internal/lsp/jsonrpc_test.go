package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestConn_SendReceive(t *testing.T) {
	// Create a pipe for testing
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	defer serverReader.Close()
	defer clientWriter.Close()
	defer clientReader.Close()
	defer serverWriter.Close()

	// Client connection
	conn := NewConn(clientReader, clientWriter)

	// Start read loop
	go conn.ReadLoop()

	// Channel to signal server completion
	serverDone := make(chan struct{})

	// Mock server: read request, send response
	go func() {
		defer close(serverDone)

		// Use bufio to properly read the LSP message
		reader := bufio.NewReader(serverReader)

		// Read headers
		var contentLength int
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Logf("server: error reading header: %v", err)
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "Content-Length:") {
				fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			}
		}

		if contentLength == 0 {
			t.Log("server: no content length found")
			return
		}

		// Read body
		body := make([]byte, contentLength)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			t.Logf("server: error reading body: %v", err)
			return
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			t.Logf("server: error unmarshaling: %v", err)
			return
		}

		// Send response
		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		result := map[string]string{"echo": "hello"}
		resultBytes, _ := json.Marshal(result)
		resp.Result = resultBytes

		respBytes, _ := json.Marshal(resp)
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(respBytes))
		serverWriter.Write([]byte(header))
		serverWriter.Write(respBytes)
	}()

	// Make a call with a timeout
	done := make(chan error, 1)
	var result map[string]string
	go func() {
		done <- conn.Call("test/method", map[string]string{"msg": "hello"}, &result)
	}()

	// Wait for result with timeout
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Call timed out")
	}

	if result["echo"] != "hello" {
		t.Errorf("unexpected result: %v", result)
	}

	conn.Close()
}

func TestConn_ReadResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantID  int64
		wantErr bool
	}{
		{
			name:   "valid response",
			input:  "Content-Length: 38\r\n\r\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":null}",
			wantID: 1,
		},
		{
			name:    "missing content length",
			input:   "\r\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":null}",
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   "Content-Length: 10\r\n\r\n{invalid}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			conn := NewConn(reader, &bytes.Buffer{})

			resp, err := conn.readResponse()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.ID != tt.wantID {
				t.Errorf("got ID %d, want %d", resp.ID, tt.wantID)
			}
		})
	}
}

func TestConn_Send(t *testing.T) {
	var buf bytes.Buffer
	conn := NewConn(strings.NewReader(""), &buf)

	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/method",
		Params:  map[string]string{"key": "value"},
	}

	err := conn.send(req)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	output := buf.String()

	// Check header
	if !strings.HasPrefix(output, "Content-Length:") {
		t.Errorf("missing Content-Length header: %s", output)
	}

	// Extract body
	idx := strings.Index(output, "\r\n\r\n")
	if idx < 0 {
		t.Fatalf("no header separator found: %s", output)
	}
	body := output[idx+4:]

	// Parse body
	var parsed Request
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("parse body: %v", err)
	}

	if parsed.Method != "test/method" {
		t.Errorf("got method %q, want %q", parsed.Method, "test/method")
	}
}

func TestResponseError(t *testing.T) {
	err := &ResponseError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "-32600") {
		t.Errorf("error string should contain code: %s", errStr)
	}
	if !strings.Contains(errStr, "Invalid Request") {
		t.Errorf("error string should contain message: %s", errStr)
	}
}

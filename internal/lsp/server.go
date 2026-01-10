package lsp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ServerConfig describes how to start an LSP server.
type ServerConfig struct {
	Command string   // e.g., "gopls", "rust-analyzer"
	Args    []string // e.g., ["serve"] for gopls
	WorkDir string   // Working directory for the server
}

// Server manages an LSP server process.
type Server struct {
	config  ServerConfig
	cmd     *exec.Cmd
	conn    *Conn
	readErr chan error
}

// StartServer starts an LSP server with the given configuration.
func StartServer(ctx context.Context, config ServerConfig) (*Server, error) {
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)
	cmd.Dir = config.WorkDir
	cmd.Stderr = os.Stderr // Pass through server errors for debugging

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting server process: %w", err)
	}

	conn := NewConn(stdout, stdin)
	readErr := make(chan error, 1)

	// Start reading responses in background
	go func() {
		readErr <- conn.ReadLoop()
	}()

	return &Server{
		config:  config,
		cmd:     cmd,
		conn:    conn,
		readErr: readErr,
	}, nil
}

// Conn returns the JSON-RPC connection for making calls.
func (s *Server) Conn() *Conn {
	return s.conn
}

// Stop gracefully stops the LSP server.
func (s *Server) Stop() error {
	s.conn.Close()

	// Wait for the process to exit
	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		// If interrupt fails, kill it
		_ = s.cmd.Process.Kill()
	}

	// Wait returns an error if the process exits with non-zero or signal,
	// but that's expected during shutdown so we ignore it.
	_ = s.cmd.Wait()
	return nil
}

// Wait waits for the server process to exit and returns any error.
func (s *Server) Wait() error {
	return s.cmd.Wait()
}

// ReadError returns the channel that receives read loop errors.
func (s *Server) ReadError() <-chan error {
	return s.readErr
}

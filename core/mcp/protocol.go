// Package mcp is Evomem's Model Context Protocol server: JSON-RPC over stdio,
// written against the specification rather than an SDK (ADR-0007).
//
// It speaks two eras. The 2026-07-28 revision removed the initialize handshake
// and made the protocol stateless: every request carries its protocol version
// and the client's capabilities in _meta, and server/discover is mandatory.
// Earlier revisions open with initialize and keep the negotiated version for
// the life of the process.
//
// Both are implemented because the specification's own compatibility matrix
// says a dual-era server is the only configuration that works with every
// client era. A modern-only server fails a legacy client with no way for it to
// fall forward.
//
// Verified against the specification on 2026-10-06:
// https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// The protocol revisions this server implements.
const (
	// Modern is the stateless revision: version and capabilities per
	// request, no handshake.
	Modern = "2026-07-28"
	// Legacy is the newest handshake-based revision.
	Legacy = "2025-11-25"
)

// Supported is what an UnsupportedProtocolVersionError advertises, newest
// first.
var Supported = []string{Modern, Legacy}

// Error codes. The standard JSON-RPC ones, plus the MCP range. The
// specification reserves -32020 to -32099 for itself and forbids inventing
// codes inside it.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602

	codeUnsupportedProtocolVersion = -32022
)

// Meta keys the specification reserves for per-request protocol fields.
const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientInfo         = "io.modelcontextprotocol/clientInfo"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"
)

// maxLineBytes bounds one incoming message. A note's content is arbitrary
// text and a project's context is many of them, so the default scanner buffer
// is far too small; a silently truncated line would reach the parser as
// invalid JSON and look like a client bug.
const maxLineBytes = 8 << 20

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// isNotification reports whether no response may be sent. A JSON-RPC
// notification has no id; the specification also forbids a null one.
func (r *request) isNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

type params struct {
	Meta map[string]json.RawMessage `json:"_meta"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return e.Message }

func errf(code int, format string, args ...any) *rpcError {
	return &rpcError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Implementation is the name and version of one side of the connection.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Serve reads newline-delimited JSON-RPC from in and writes replies to out
// until in reaches end of file.
//
// Nothing but protocol messages may reach out: the specification is explicit,
// and a stray print there corrupts the stream for the client. Anything the
// server wants to say goes to stderr.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	reader := bufio.NewScanner(in)
	reader.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		if reply := s.handleLine([]byte(line)); reply != nil {
			if err := writeMessage(writer, reply); err != nil {
				return err
			}
		}
	}
	return reader.Err()
}

// writeMessage emits one message per line. A message containing a newline
// would break the framing for everything after it, so the encoder's own
// trailing newline is the only one allowed.
func writeMessage(w *bufio.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		return err
	}
	return w.Flush()
}

// handleLine turns one incoming line into the reply to send, or nil when none
// is owed.
func (s *Server) handleLine(line []byte) any {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, errf(codeParse, "invalid JSON"))
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		if req.isNotification() {
			return nil
		}
		return errorResponse(req.ID, errf(codeInvalidRequest, "not a JSON-RPC 2.0 request"))
	}

	if req.isNotification() {
		s.handleNotification(req)
		return nil
	}

	result, era, rpcErr := s.dispatch(req)
	if rpcErr != nil {
		return errorResponse(req.ID, rpcErr)
	}
	return resultResponse(req.ID, result, era, s.info)
}

// handleNotification swallows the notifications a client may send. None of
// them needs an answer, and an unknown one is not an error.
func (s *Server) handleNotification(req request) {
	if req.Method == "notifications/initialized" {
		s.legacy = true
	}
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func errorResponse(id json.RawMessage, e *rpcError) response {
	return response{JSONRPC: "2.0", ID: id, Error: e}
}

// resultResponse wraps a result for the era it is answering.
//
// A modern result must carry resultType, and should carry the server's
// identity in _meta. A legacy client predates both: resultType is harmless
// there but serverInfo in _meta is not where it looks for it, so neither is
// sent.
func resultResponse(id json.RawMessage, result map[string]any, era string, info Implementation) response {
	if result == nil {
		result = map[string]any{}
	}
	if era == Modern {
		result["resultType"] = "complete"
		result["_meta"] = map[string]any{metaServerInfo: info}
	}
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

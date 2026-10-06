package mcp

import (
	"context"
	"encoding/json"

	"github.com/forgeprint/evomem/shared/database"
)

// Server answers one stdio connection over a store.
type Server struct {
	db   *database.DB
	info Implementation

	// ctx bounds every query the tools run. The transport has no request
	// scope of its own, so the process's context is the only one there is.
	ctx context.Context

	// legacy records that this process has been opened with an initialize
	// handshake. It is the one piece of connection state the server keeps,
	// and only the legacy era is entitled to it: the modern protocol
	// forbids inferring anything from a previous request, which is why
	// every modern request is checked on its own below.
	legacy bool
}

// New returns a server over a store.
func New(ctx context.Context, db *database.DB, version string) *Server {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Server{
		db:   db,
		info: Implementation{Name: "evomem", Version: version},
		ctx:  ctx,
	}
}

// dispatch routes one request and returns the result together with the era it
// was answered in.
func (s *Server) dispatch(req request) (map[string]any, string, *rpcError) {
	// An initialize request is how a legacy client opens, and the only
	// thing that puts this process into the legacy era.
	if req.Method == "initialize" {
		s.legacy = true
		result, err := s.initialize(req.Params)
		return result, Legacy, err
	}

	era, err := s.era(req.Params)
	if err != nil {
		return nil, "", err
	}

	switch req.Method {
	case "server/discover":
		return s.discover(), era, nil
	case "tools/list":
		return s.listTools(), era, nil
	case "tools/call":
		result, err := s.callTool(req.Params)
		return result, era, err
	case "ping":
		return map[string]any{}, era, nil
	default:
		return nil, "", errf(codeMethodNotFound, "unknown method %q", req.Method)
	}
}

// era decides which revision a request is speaking, and rejects it when the
// answer is neither.
func (s *Server) era(raw json.RawMessage) (string, *rpcError) {
	var p params
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", errf(codeInvalidParams, "params is not an object")
		}
	}

	version := ""
	if v, ok := p.Meta[metaProtocolVersion]; ok {
		_ = json.Unmarshal(v, &version)
	}

	if version == "" {
		// No per-request version. Only a client that has already shaken
		// hands is entitled to that.
		if s.legacy {
			return Legacy, nil
		}
		return "", errf(codeInvalidParams,
			"_meta.%s is required; a legacy client must send initialize first", metaProtocolVersion)
	}

	if version != Modern && version != Legacy {
		// The specification's own shape for this, so a modern client
		// knows to retry rather than to fall back to initialize.
		return "", &rpcError{
			Code:    codeUnsupportedProtocolVersion,
			Message: "Unsupported protocol version",
			Data:    map[string]any{"supported": Supported, "requested": version},
		}
	}

	// clientCapabilities is required on every modern request. A server
	// must not assume a capability the client has not declared, so an
	// absent field is malformed rather than an empty set.
	if _, ok := p.Meta[metaClientCapabilities]; !ok {
		return "", errf(codeInvalidParams, "_meta.%s is required", metaClientCapabilities)
	}
	return Modern, nil
}

// discover is how a modern client learns what this server speaks without
// committing to a version. The specification requires every server to
// implement it.
func (s *Server) discover() map[string]any {
	return map[string]any{
		"supportedVersions": Supported,
		"serverInfo":        s.info,
		"capabilities":      s.capabilities(),
	}
}

// capabilities says what this server offers. Tools and nothing else: no
// resources, no prompts, no sampling.
//
// listChanged is false because the tool set is fixed at build time. Saying
// otherwise would promise notifications that never come.
func (s *Server) capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{"listChanged": false}}
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// initialize answers a legacy client.
//
// The version echoed back is the client's when this server speaks it, and the
// newest legacy revision otherwise; a legacy client has no way to fall
// forward, so the reply has to be something it can work with or an error it
// can show a person.
func (s *Server) initialize(raw json.RawMessage) (map[string]any, *rpcError) {
	var p initializeParams
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p)
	}

	version := Legacy
	if p.ProtocolVersion == Legacy {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    s.capabilities(),
		"serverInfo":      s.info,
	}, nil
}

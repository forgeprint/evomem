package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/forgeprint/evomem/core/api"
	"github.com/forgeprint/evomem/core/connect"
	"github.com/forgeprint/evomem/core/organize"
	"github.com/forgeprint/evomem/shared/database"
)

// cmdServe starts the HTTP server.
func cmdServe(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	cfg := api.Config{
		Addr:                 getEnv("EVOMEM_ADDR", *addr),
		Token:                os.Getenv("EVOMEM_API_TOKEN"),
		DefaultProject:       os.Getenv("EVOMEM_PROJECT"),
		TelegramSecret:       os.Getenv("EVOMEM_TELEGRAM_SECRET"),
		TelegramProject:      os.Getenv("EVOMEM_TELEGRAM_PROJECT"),
		TelegramChatProjects: parseChatProjects(os.Getenv("EVOMEM_TELEGRAM_CHAT_PROJECTS")),
		JiraSecret:           os.Getenv("EVOMEM_JIRA_SECRET"),
		JiraProject:          os.Getenv("EVOMEM_JIRA_PROJECT"),
		CORSOrigins:          os.Getenv("EVOMEM_CORS_ORIGIN"),
		// One line per request, on stderr with everything else the
		// server says about itself. EVOMEM_ACCESS_LOG=off turns it
		// off for somebody who would rather not keep one (ADR-0035).
		AccessLog: accessLog(),
		SecretKey: os.Getenv("EVOMEM_SECRET_KEY"),
		Pull: func(ctx context.Context, key database.SecretKey) (connect.Result, error) {
			// The panel asks; this is where the connectors are named.
			return connect.Run(ctx, db, key, map[string]connect.Puller{
				"jira": connect.NewJiraPuller(),
			}, io.Discard)
		},
		Organize: func(ctx context.Context, key database.SecretKey, project string, dryRun bool) (api.OrganizeResult, error) {
			// The panel asks; this is where the model is named.
			connections, err := db.Connections(ctx)
			if err != nil {
				return api.OrganizeResult{}, err
			}
			model, err := organize.FromConnection(connections, key)
			if errors.Is(err, organize.ErrNotConfigured) {
				return api.OrganizeResult{}, api.ErrNoModel
			}
			if err != nil {
				return api.OrganizeResult{}, err
			}
			r, err := organize.Run(ctx, db, model, project, dryRun, io.Discard)
			return api.OrganizeResult{
				Offered: r.Offered, Created: r.Created, Grouped: r.Grouped,
				Invented: r.Invented, Dropped: r.Dropped,
			}, err
		},
		Recordings: recordings(),
	}

	server, err := api.New(db, cfg)
	if err != nil {
		return err
	}

	// Which memory this is serving, for the same reason the MCP server
	// says it: two processes on one machine can quietly open two
	// different stores.
	fmt.Fprintf(out, "store: %s\n", db.Path())
	fmt.Fprintf(out, "serving on %s\n", server.Addr())
	for _, r := range server.Routes() {
		fmt.Fprintf(out, "  %s\n", r)
	}

	ctx := context.Background()
	return server.ListenAndServe(ctx)
}

func parseChatProjects(s string) api.ChatProjectMap {
	if s == "" {
		return nil
	}
	var m api.ChatProjectMap
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		// Invalid JSON falls back to empty map
		return nil
	}
	return m
}

// getEnv prefers the environment, falling back to a flag's value.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// accessLog is where request lines go: stderr, or nowhere when
// EVOMEM_ACCESS_LOG is "off".
//
// On by default. The cost of keeping it is a line per request on stderr; the
// cost of not keeping it was a day when ten notes were deleted through the
// API and the only honest answer to "by whom" was that nothing had recorded
// it.
func accessLog() io.Writer {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("EVOMEM_ACCESS_LOG")), "off") {
		return nil
	}
	return os.Stderr
}

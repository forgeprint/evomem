package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/forgeprint/evomem/core/api"
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
	}

	server, err := api.New(db, cfg)
	if err != nil {
		return err
	}

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

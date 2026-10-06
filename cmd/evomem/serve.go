package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/forgeprint/evomem/core/api"
)

// cmdServe runs the HTTP ingestion endpoint.
//
// Every secret comes from the environment rather than a flag. A flag is
// visible in ps to every process on the machine, and these are the only thing
// standing between the store and whoever can reach the port.
func cmdServe(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "address to listen on (default "+api.DefaultAddr+")")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := api.Config{
		Addr:            *addr,
		Token:           os.Getenv("EVOMEM_API_TOKEN"),
		DefaultProject:  os.Getenv("EVOMEM_PROJECT"),
		TelegramSecret:  os.Getenv("EVOMEM_TELEGRAM_SECRET"),
		TelegramProject: os.Getenv("EVOMEM_TELEGRAM_PROJECT"),
		JiraSecret:      os.Getenv("EVOMEM_JIRA_SECRET"),
		JiraProject:     os.Getenv("EVOMEM_JIRA_PROJECT"),
	}
	if cfg.Addr == "" {
		cfg.Addr = os.Getenv("EVOMEM_ADDR")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	server, err := api.New(db, cfg)
	if err != nil {
		return err
	}

	// What is served, and nothing about the secrets themselves. An
	// endpoint missing because its secret was not set is the mistake this
	// output is here to make obvious.
	fmt.Fprintf(out, "evomem serve: %s, store %s\n", server.Addr(), db.Path())
	for _, route := range server.Routes() {
		fmt.Fprintf(out, "  %s\n", route)
	}
	fmt.Fprintln(out, "  GET /healthz")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.ListenAndServe(ctx)
}

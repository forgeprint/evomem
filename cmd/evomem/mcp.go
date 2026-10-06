package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/forgeprint/evomem/core/mcp"
)

// cmdMCP serves the Model Context Protocol on stdin and stdout.
//
// out is not where the protocol goes. A coding agent starts this as a
// subprocess and reads its stdout as the message stream, so stdout belongs to
// the protocol alone and everything else — including the one line saying the
// server started — goes to stderr. `run` is given a writer for ordinary
// command output, and using it here would corrupt the stream.
func cmdMCP(args []string, _ io.Writer) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	quiet := fs.Bool("quiet", false, "do not announce the store on stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	if !*quiet {
		fmt.Fprintf(os.Stderr, "evomem mcp: serving %s\n", db.Path())
	}

	server := mcp.New(context.Background(), db, version)
	return server.Serve(os.Stdin, os.Stdout)
}

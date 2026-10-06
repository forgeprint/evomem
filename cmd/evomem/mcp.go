package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/forgeprint/evomem/core/mcp"
)

// cmdMCP serves the Model Context Protocol on the streams it is given, which
// in main are stdin and stdout.
//
// For this one command the streams are the protocol. A coding agent starts it
// as a subprocess and reads its stdout as the message stream, so nothing but
// protocol messages may go there — the line saying which store is being served
// goes to stderr, and so would anything else this command wanted to say.
//
// Taking the streams as arguments rather than reaching for os.Stdout is what
// makes the server testable from the command's own entry point: a test can
// feed it a request and read the reply.
func cmdMCP(args []string, out io.Writer, in io.Reader) error {
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
	return server.Serve(in, out)
}

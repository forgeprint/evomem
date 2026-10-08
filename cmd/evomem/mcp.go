package main

import (
	"flag"
	"fmt"
	"io"
)

// cmdMCP is the entrypoint for the MCP server.
func cmdMCP(args []string, out io.Writer, in io.Reader) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	quiet := fs.Bool("quiet", false, "suppress stderr output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Fprintln(out, "MCP server not yet implemented")
	return nil
}
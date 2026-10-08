// cmdServe starts the HTTP server.
func cmdServe(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Fprintln(out, "HTTP server not yet implemented")
	return nil
}
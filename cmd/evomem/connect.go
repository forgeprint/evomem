package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/forgeprint/evomem/core/connect"
	"github.com/forgeprint/evomem/shared/database"
)

// secretKey reads the key the stored tokens are sealed with.
//
// From the environment, as ADR-0010 requires of every secret. Without it the
// connections cannot be read and nothing can be pulled, which is said rather
// than failing obscurely.
func secretKey() (database.SecretKey, error) {
	return database.ParseSecretKey(os.Getenv("EVOMEM_SECRET_KEY"))
}

// cmdConnect manages the sources this hub pulls from.
//
// A token is read from stdin, never from a flag: a flag is visible in `ps` to
// every process on the machine (ADR-0010).
func cmdConnect(args []string, out io.Writer, in io.Reader) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	add := fs.String("add", "", "add a source: jira, or a model key: model")
	remove := fs.String("remove", "", "forget a connection, and its token, by id")
	project := fs.String("project", "", "the project pulled notes are filed under")
	baseURL := fs.String("url", "", "the source's base address, e.g. https://you.atlassian.net")
	account := fs.String("account", "", "the account the token belongs to; Jira wants the email")
	query := fs.String("query", "", "what to ask the source for; Jira takes JQL, a model takes its name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	if *remove != "" {
		if err := db.RemoveConnection(ctx, *remove); err != nil {
			if errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("there is no connection %s", *remove)
			}
			return err
		}
		fmt.Fprintf(out, "forgot %s, and the token with it\n", *remove)
		return nil
	}

	if *add != "" {
		key, err := secretKey()
		if err != nil {
			return fmt.Errorf("%w\n\nSet EVOMEM_SECRET_KEY to 32 bytes and keep it: it is what\n"+
				"seals the tokens, and losing it means entering them again", err)
		}

		fmt.Fprintf(out, "Paste the API token for %s and press enter.\n", *add)
		fmt.Fprintln(out, "It is read from stdin rather than a flag, which `ps` would show.")
		secret, err := readLine(in)
		if err != nil {
			return err
		}

		conn := &database.Connection{
			SourceType: *add,
			ProjectID:  *project,
			BaseURL:    *baseURL,
			Account:    *account,
			Query:      *query,
		}
		if err := db.AddConnection(ctx, conn, secret, key); err != nil {
			return err
		}
		fmt.Fprintf(out, "\nconnected %s as %s\n", conn.SourceType, conn.ID)
		if conn.IsModel() {
			fmt.Fprintln(out, "evomem organize       asks it to group the notes nothing has grouped")
			fmt.Fprintln(out, "\nPressing that sends the text of those notes to", conn.BaseURL)
		} else {
			fmt.Fprintln(out, "evomem pull-sources   pulls from it")
		}
		return nil
	}

	connections, err := db.Connections(ctx)
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		fmt.Fprintln(out, "no sources connected")
		fmt.Fprintln(out, "\nevomem connect -add jira -project <id> -url https://you.atlassian.net \\")
		fmt.Fprintln(out, "  -account you@example.com -query 'project = EVO ORDER BY updated DESC'")
		return nil
	}

	// "connection(s)", not "source(s)": the keyring holds model keys too
	// and they are not sources (ADR-0027).
	fmt.Fprintf(out, "%d connection(s):\n", len(connections))
	for _, conn := range connections {
		fmt.Fprintf(out, "\n%s  %s  -> %s\n  %s", conn.ID, conn.SourceType, conn.ProjectID, conn.BaseURL)
		if conn.Account != "" {
			fmt.Fprintf(out, "  as %s", conn.Account)
		}
		fmt.Fprintln(out)
		if conn.IsModel() {
			// Not a source: the query column holds which model
			// (ADR-0027), and "asks for" would read as a search.
			fmt.Fprintf(out, "  groups notes with: %s\n", conn.ModelName())
		} else if conn.Query != "" {
			fmt.Fprintf(out, "  asks for: %s\n", conn.Query)
		}
		if conn.LastPulledAt != nil {
			fmt.Fprintf(out, "  last pulled %s\n", conn.LastPulledAt.Format("2006-01-02 15:04"))
		}
		if conn.LastError != "" {
			fmt.Fprintf(out, "  last failure: %s\n", conn.LastError)
		}
	}
	return nil
}

// cmdPullSources calls every connected source.
func cmdPullSources(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("pull-sources", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	connections, err := db.Connections(ctx)
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		fmt.Fprintln(out, "no sources connected; see evomem connect")
		return nil
	}

	key, err := secretKey()
	if err != nil {
		return fmt.Errorf("%w\n\nThe tokens are sealed with it; without it nothing can be pulled", err)
	}

	result, err := connect.Run(ctx, db, key, map[string]connect.Puller{
		"jira": connect.NewJiraPuller(),
	}, out)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\n%d source(s), %d note(s) pulled, %d failed\n",
		result.Considered, result.Pulled, result.Failed)
	if result.Failed > 0 {
		fmt.Fprintln(out, "A failure is recorded on the connection; see evomem connect.")
	}
	return nil
}

// readLine reads one line, which is how a token arrives.
func readLine(in io.Reader) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(b.String()), nil
}

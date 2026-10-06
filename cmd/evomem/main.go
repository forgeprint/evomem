// Command evomem is the local entrypoint to the store.
//
// It exists at this stage so that the storage layer can be exercised by hand
// as well as by its tests. The MCP server and the HTTP entrypoint are
// subcommands added in later phases.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// version is set by the linker in scripts/build.sh.
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "evomem:", err)
		os.Exit(1)
	}
}

const usage = `evomem - local-first memory for coding agents

usage:
  evomem version
  evomem init
  evomem add    -project <id> [-source <type>] [<text> | -]
  evomem search -query <text> [-project <id>] [-limit <n>]
  evomem list   [-project <id>] [-limit <n>]
  evomem delete <id>
  evomem projects
  evomem review [-accept <id> | -reject <id>] [-status <s>]
  evomem mcp    [-quiet]
  evomem serve  [-addr <host:port>]
  evomem sync   [-once] [-init-remote] [-interval <d>]
  evomem sync-status
  evomem archive [-months <n>] [-project <id>] [-vacuum] [-dry-run]

The mcp command speaks the Model Context Protocol on stdin and stdout; it is
what a coding agent starts, not something to run by hand. An agent can propose
a note through it, and review is where a person accepts or turns one down:
nothing an agent proposes is in memory until then.

The store is $EVOMEM_DB, or ~/.evomem/evomem.db.

serve reads its secrets from the environment, never from a flag:
  EVOMEM_API_TOKEN        bearer token for POST /ingest
  EVOMEM_PROJECT          project /ingest files a note under by default
  EVOMEM_TELEGRAM_SECRET  setWebhook's secret_token
  EVOMEM_TELEGRAM_PROJECT project Telegram notes are filed under
  EVOMEM_JIRA_SECRET      the Jira webhook's secret
  EVOMEM_JIRA_PROJECT     overrides the issue's own project key
  EVOMEM_ADDR             address to listen on

sync reads its connection string from the environment for the same reason:
  EVOMEM_POSTGRES_DSN     postgres://user:password@host:5432/db

An endpoint whose secret is unset is not served at all.
`

func run(args []string, out io.Writer, in io.Reader) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return nil
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version":
		fmt.Fprintln(out, version)
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	case "init":
		return cmdInit(out)
	case "add":
		return cmdAdd(rest, out, in)
	case "search":
		return cmdSearch(rest, out)
	case "list":
		return cmdList(rest, out)
	case "projects":
		return cmdProjects(out)
	case "delete":
		return cmdDelete(rest, out)
	case "review":
		return cmdReview(rest, out)
	case "mcp":
		return cmdMCP(rest, out, in)
	case "serve":
		return cmdServe(rest, out)
	case "archive":
		return cmdArchive(rest, out)
	case "sync-status":
		return cmdSyncStatus(rest, out)
	case "sync":
		return cmdSync(rest, out)
	default:
		return fmt.Errorf("unknown command %q; run evomem help", cmd)
	}
}

// storePath is where the store lives. One environment variable rather than a
// global flag, so a shell can point a whole session at a scratch database.
func storePath() (string, error) {
	if p := os.Getenv("EVOMEM_DB"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".evomem", "evomem.db"), nil
}

func openStore() (*database.DB, error) {
	path, err := storePath()
	if err != nil {
		return nil, err
	}
	return database.Open(path)
}

func cmdInit(out io.Writer) error {
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Fprintf(out, "store ready at %s\n", db.Path())
	return nil
}

func cmdAdd(args []string, out io.Writer, in io.Reader) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	project := fs.String("project", "", "project the note belongs to")
	source := fs.String("source", string(models.SourceManual), "what produced the note")
	if err := fs.Parse(args); err != nil {
		return err
	}

	content, err := readContent(fs.Args(), in)
	if err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	n := &models.Note{
		ProjectID:  *project,
		Content:    content,
		SourceType: models.SourceType(*source),
	}
	if err := db.Create(context.Background(), n); err != nil {
		return err
	}
	fmt.Fprintln(out, n.ID)
	return nil
}

// readContent takes the note text from the arguments, or from stdin when the
// only argument is "-". Piping is how an Apple Shortcut or a shell one-liner
// will use this.
func readContent(args []string, in io.Reader) (string, error) {
	if len(args) == 1 && args[0] == "-" {
		data, err := io.ReadAll(in)
		if err != nil {
			return "", fmt.Errorf("reading standard input: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if len(args) == 0 {
		return "", errors.New("no text given; pass it as an argument or pipe it with -")
	}
	return strings.Join(args, " "), nil
}

func cmdSearch(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	query := fs.String("query", "", "what to search for")
	project := fs.String("project", "", "narrow to one project")
	limit := fs.Int("limit", 20, "how many hits at most")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *query == "" {
		return errors.New("-query is required")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	hits, err := db.Search(context.Background(), database.SearchQuery{
		Text:      *query,
		ProjectID: *project,
		Limit:     *limit,
	})
	if err != nil {
		return err
	}
	if hits == nil {
		hits = []database.SearchHit{}
	}
	return writeJSON(out, hits)
}

func cmdList(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	project := fs.String("project", "", "narrow to one project")
	source := fs.String("source", "", "narrow to one source")
	limit := fs.Int("limit", 20, "how many notes at most")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	notes, err := db.List(context.Background(), database.ListOptions{
		ProjectID:  *project,
		SourceType: models.SourceType(*source),
		Limit:      *limit,
	})
	if err != nil {
		return err
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	return writeJSON(out, notes)
}

// cmdDelete removes one note.
//
// It leaves a tombstone, which is what lets the deletion reach the remote
// copy. Without one the row would simply be absent, and absent is
// indistinguishable from never written.
func cmdDelete(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("delete takes one note id")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Delete(context.Background(), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(out, "deleted %s\n", args[0])
	return nil
}

func cmdProjects(out io.Writer) error {
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	projects, err := db.Projects(context.Background())
	if err != nil {
		return err
	}
	if projects == nil {
		projects = []database.ProjectSummary{}
	}
	return writeJSON(out, projects)
}

// writeJSON is the only output format for anything structured. A note's
// content is arbitrary text, so a column layout would need escaping rules
// that JSON already has.
//
// Callers pass an empty slice rather than a nil one: a consumer parsing this
// should get [] for no results, never null.
func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

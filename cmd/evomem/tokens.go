package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/forgeprint/evomem/shared/database"
)

// cmdTokens manages which devices can reach the server (ADR-0030).
//
// Evomem has no accounts and no login: this is one person's memory on their
// own machine, and a token is how a device says it is theirs. The point of
// having more than one is the unit of revocation — losing a phone should
// cost a phone.
func cmdTokens(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tokens", flag.ContinueOnError)
	add := fs.String("add", "", "mint a token for a device, under this name")
	revoke := fs.String("revoke", "", "turn a token off, by id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	if *revoke != "" {
		if err := db.RevokeToken(ctx, *revoke); err != nil {
			if errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("there is no token %s", *revoke)
			}
			return err
		}
		fmt.Fprintf(out, "revoked %s\n", *revoke)
		fmt.Fprintln(out, "Whatever was using it is refused from its next request.")
		return nil
	}

	if *add != "" {
		token, secret, err := db.NewToken(ctx, *add)
		if err != nil {
			if errors.Is(err, database.ErrEmptyTokenName) {
				return errors.New("a token needs a name, so it can be told from the others")
			}
			return err
		}
		fmt.Fprintf(out, "%s  %s\n\n", token.ID, token.Name)
		fmt.Fprintln(out, secret)
		// Said plainly, because it is true: the store holds a hash.
		fmt.Fprintln(out, "\nThat is the only time this is shown. Nothing can read it back;")
		fmt.Fprintln(out, "if it is lost, revoke it and make another.")
		return nil
	}

	tokens, err := db.Tokens(ctx)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		fmt.Fprintln(out, "no device tokens; EVOMEM_API_TOKEN is the only way in")
		fmt.Fprintln(out, "\nevomem tokens -add phone")
		return nil
	}

	fmt.Fprintf(out, "%d token(s):\n", len(tokens))
	for _, token := range tokens {
		fmt.Fprintf(out, "\n%s  %s  (evm_%s_…)\n", token.ID, token.Name, token.Prefix)
		fmt.Fprintf(out, "  made %s\n", token.CreatedAt.Format("2006-01-02 15:04"))
		if token.LastUsedAt != nil {
			// Why this is worth showing: it is what makes revoking
			// safe rather than a guess.
			fmt.Fprintf(out, "  last used %s\n", token.LastUsedAt.Format("2006-01-02 15:04"))
		} else {
			fmt.Fprintln(out, "  never used")
		}
		if token.Revoked() {
			fmt.Fprintf(out, "  REVOKED %s\n", token.RevokedAt.Format("2006-01-02 15:04"))
		}
	}
	fmt.Fprintln(out, "\nevomem tokens -revoke <id>   turns one off")
	return nil
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/forgeprint/evomem/shared/audio"
)

// cmdAudio converts recordings already on disk into the one format every
// client can play.
//
// Uploads arriving from now on are converted as they land (ADR-0036); this
// is for the ones that arrived before, and for a store that was carried over
// from a machine with no ffmpeg. It says what it would do unless told to do
// it, because rewriting somebody's recordings is not a thing to do by
// accident.
func cmdAudio(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("audio", flag.ContinueOnError)
	fs.SetOutput(out)
	convert := fs.Bool("convert", false, "convert the recordings that need it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store := recordings()
	names, err := needConverting(store.Dir())
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "recordings in %s\n", store.Dir())
	if len(names) == 0 {
		fmt.Fprintln(out, "nothing to convert: every recording is already playable everywhere")
		return nil
	}

	if !*convert {
		fmt.Fprintf(out, "%d recording(s) would be converted to %s:\n", len(names), audio.PlayableExt)
		for _, name := range names {
			fmt.Fprintf(out, "  %s\n", filepath.Base(name))
		}
		fmt.Fprintln(out, "\nevomem audio -convert   does it")
		return nil
	}

	converter := audio.FFmpeg{}
	if !converter.Available() {
		return fmt.Errorf("%w\n\nThe server's image carries one; a local evomem has whatever the machine has", audio.ErrNoConverter)
	}

	var done, failed int
	for _, name := range names {
		target := strings.TrimSuffix(name, filepath.Ext(name)) + audio.PlayableExt
		if err := converter.Convert(context.Background(), name, target); err != nil {
			fmt.Fprintf(out, "  %s stays as it is: %v\n", filepath.Base(name), err)
			failed++
			continue
		}
		// Only after the new one exists: until then the original is the
		// only copy there is.
		if err := os.Remove(name); err != nil {
			return fmt.Errorf("removing %s after converting it: %w", name, err)
		}
		fmt.Fprintf(out, "  %s -> %s\n", filepath.Base(name), filepath.Base(target))
		done++
	}

	fmt.Fprintf(out, "\n%d converted, %d left as they were\n", done, failed)
	if failed > 0 {
		return errors.New("some recordings could not be converted")
	}
	return nil
}

// needConverting lists the stored recordings that not every client can play.
func needConverting(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		// No recording has ever been saved. Not an error.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !audio.NeedsConverting(entry.Name()) {
			continue
		}
		names = append(names, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(names)
	return names, nil
}

package audio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PlayableExt is what every client this project has can play.
//
// AAC in an MP4 container. A phone records it natively, a browser plays it,
// and `shared/audio` already maps it. Nothing else is true of all three:
// Opus in WebM is what a browser records and what AVFoundation has no
// demuxer for, so a note recorded in a browser was unplayable on a phone
// (ADR-0029, ADR-0031).
const PlayableExt = ".m4a"

// convertible is what is worth converting: the containers Apple platforms
// cannot read.
//
// Not a list of "everything that is not m4a". An mp3 or a wav plays
// everywhere already, and re-encoding one would lose quality to no purpose.
var convertible = map[string]bool{
	".webm": true,
	".ogg":  true,
}

// NeedsConverting reports whether a stored recording should be converted for
// everything to be able to play it.
func NeedsConverting(name string) bool {
	return convertible[strings.ToLower(filepath.Ext(name))]
}

// ErrNoConverter is returned when nothing on this machine can convert.
//
// An ordinary outcome, not a failure of the upload: `evomem` is one static
// binary with no runtime dependencies (CLAUDE.md), so a local install very
// likely has no ffmpeg. The recording is kept exactly as it arrived and the
// clients that can play it still can.
var ErrNoConverter = errors.New("audio: nothing here can convert a recording")

// A Converter turns a recording into the one format every client can play.
type Converter interface {
	// Convert reads src and writes dst. Both are paths: an MP4 muxer needs
	// to seek, so a pipe would need fragmenting and a file does not.
	Convert(ctx context.Context, src, dst string) error
}

// FFmpeg converts by running ffmpeg.
//
// Verified against ffmpeg 6.1.2 in alpine:3.22 on 2026-10-10 by converting a
// real browser recording — Opus in WebM, 48 kHz — and measuring the result:
// AAC in m4a, mono at 16 kHz, 10.05s, peak amplitude 4331 against the
// original's 4348. The audio survives; the container and codec change.
type FFmpeg struct {
	// Bin is the program to run. Empty means look for "ffmpeg" on PATH.
	Bin string

	// Timeout bounds one conversion. Zero means DefaultConvertTimeout.
	Timeout time.Duration
}

// DefaultConvertTimeout bounds one conversion.
//
// Generous: a recording is bounded at MaxBytes, which is roughly an hour and
// a half of speech at the settings this records, and a slow machine
// re-encoding that should not be cut off halfway. It is here so a wedged
// ffmpeg cannot hold an upload open for ever.
const DefaultConvertTimeout = 5 * time.Minute

// Available reports whether there is an ffmpeg to run.
func (f FFmpeg) Available() bool {
	_, err := exec.LookPath(f.bin())
	return err == nil
}

func (f FFmpeg) bin() string {
	if f.Bin != "" {
		return f.Bin
	}
	return "ffmpeg"
}

// Convert writes dst as AAC in an m4a.
//
// The settings match what the phone records, for the reason ADR-0029 gives:
// a transcription model resamples to 16 kHz mono before it does anything
// else, so anything more is discarded on arrival and paid for in bytes on
// the way.
func (f FFmpeg) Convert(ctx context.Context, src, dst string) error {
	if !f.Available() {
		return ErrNoConverter
	}

	timeout := f.Timeout
	if timeout <= 0 {
		timeout = DefaultConvertTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// -nostdin because this runs inside a server: an ffmpeg that decides to
	// read the terminal would hang holding a request open.
	// -vn because a WebM from MediaRecorder can carry a video stream, and a
	// recording is audio whatever else is in the file.
	cmd := exec.CommandContext(ctx, f.bin(),
		"-nostdin", "-y", "-loglevel", "error",
		"-i", src,
		"-vn",
		"-c:a", "aac", "-b:a", "32k", "-ac", "1", "-ar", "16000",
		"-movflags", "+faststart",
		dst)

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Whatever is half-written is not a recording.
		_ = os.Remove(dst)
		if ctx.Err() != nil {
			return fmt.Errorf("audio: converting %s: %w", filepath.Base(src), ctx.Err())
		}
		return fmt.Errorf("audio: converting %s: %w: %s",
			filepath.Base(src), err, strings.TrimSpace(string(out)))
	}
	return nil
}

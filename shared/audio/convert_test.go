package audio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeConverter writes a stand-in file, so a test can assert on what the
// store did without needing an ffmpeg.
type fakeConverter struct {
	calls int
	from  string
	err   error
}

func (f *fakeConverter) Convert(_ context.Context, src, dst string) error {
	f.calls++
	f.from = src
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(dst, []byte("converted"), 0o600)
}

func TestOnlyWhatApplePlatformsCannotReadIsConverted(t *testing.T) {
	// A browser records the first; Telegram sends the second. Neither has
	// a demuxer on an Apple platform (ADR-0031).
	for _, name := range []string{"x.webm", "x.ogg", "X.WEBM"} {
		if !NeedsConverting(name) {
			t.Errorf("%s should be converted", name)
		}
	}
	// These already play everywhere. Re-encoding one would lose quality
	// for nothing.
	for _, name := range []string{"x.m4a", "x.mp3", "x.wav", "x"} {
		if NeedsConverting(name) {
			t.Errorf("%s should be left alone", name)
		}
	}
}

func TestAWebmUploadIsStoredAsSomethingEveryClientPlays(t *testing.T) {
	dir := t.TempDir()
	convert := &fakeConverter{}
	store := (&Store{dir: dir}).WithConverter(convert, nil)

	const id = "01M4D3H3HNMFM69N4MHNAYBZ1X"
	name, err := store.Save(id, "audio/webm", strings.NewReader("pretend-webm"))
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Ext(name) != PlayableExt {
		t.Errorf("stored as %s, want %s", filepath.Ext(name), PlayableExt)
	}
	if convert.calls != 1 {
		t.Errorf("converted %d times", convert.calls)
	}
	// The original goes, or the note has two recordings and only one of
	// them is reachable.
	if _, err := os.Stat(filepath.Join(dir, id+".webm")); !os.IsNotExist(err) {
		t.Error("the webm is still there beside the m4a")
	}

	// And it is the m4a that is served back.
	_, stored, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(stored) != PlayableExt {
		t.Errorf("Open gives %s", stored)
	}
}

func TestAnM4aUploadIsNotTouched(t *testing.T) {
	convert := &fakeConverter{}
	store := (&Store{dir: t.TempDir()}).WithConverter(convert, nil)

	if _, err := store.Save("01M4D3H3HNMFM69N4MHNAYBZ1X", "audio/m4a",
		strings.NewReader("pretend-m4a")); err != nil {
		t.Fatal(err)
	}
	if convert.calls != 0 {
		t.Errorf("an m4a was re-encoded %d times for no reason", convert.calls)
	}
}

// Losing the audio would be worse than one client not being able to play it.
func TestAFailedConversionKeepsTheRecordingAsItArrived(t *testing.T) {
	dir := t.TempDir()
	convert := &fakeConverter{err: errors.New("ffmpeg fell over")}

	var reported error
	var reportedFrom, reportedTo string
	store := (&Store{dir: dir}).WithConverter(convert,
		func(_, from, to string, err error) {
			reportedFrom, reportedTo, reported = from, to, err
		})

	const id = "01M4D3H3HNMFM69N4MHNAYBZ1X"
	name, err := store.Save(id, "audio/webm", strings.NewReader("pretend-webm"))
	if err != nil {
		t.Fatalf("the upload failed because a conversion did: %v", err)
	}
	if filepath.Ext(name) != ".webm" {
		t.Errorf("stored as %s", name)
	}

	body, stored, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if filepath.Ext(stored) != ".webm" {
		t.Errorf("Open gives %s", stored)
	}

	// And somebody is told, or it fails silently for ever.
	if reported == nil {
		t.Error("nothing was reported")
	}
	if reportedFrom != ".webm" || reportedTo != PlayableExt {
		t.Errorf("reported %s to %s", reportedFrom, reportedTo)
	}
}

// A local evomem is one static binary with no runtime dependencies, so this
// is the ordinary case, not an error.
func TestNoConverterKeepsTheRecording(t *testing.T) {
	dir := t.TempDir()
	store := &Store{dir: dir} // No converter at all.

	const id = "01M4D3H3HNMFM69N4MHNAYBZ1X"
	name, err := store.Save(id, "audio/webm", strings.NewReader("pretend-webm"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(name) != ".webm" {
		t.Errorf("stored as %s", name)
	}
}

func TestFFmpegSaysWhenItIsNotThere(t *testing.T) {
	missing := FFmpeg{Bin: "evomem-no-such-program"}
	if missing.Available() {
		t.Fatal("a program that does not exist reported available")
	}
	if err := missing.Convert(context.Background(), "a", "b"); !errors.Is(err, ErrNoConverter) {
		t.Errorf("got %v, want ErrNoConverter", err)
	}
}

// A conversion that fails must not leave half a file where a recording goes.
func TestAFailedConversionLeavesNoHalfFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.webm")
	if err := os.WriteFile(src, []byte("not audio at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out.m4a")

	// `false` exists everywhere and fails, which is what an ffmpeg that
	// cannot decode the input does.
	err := FFmpeg{Bin: "false"}.Convert(context.Background(), src, dst)
	if err == nil {
		t.Fatal("a failing converter reported success")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("half a file was left where a recording goes")
	}
}

package audio

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const noteID = "01M4D3H3HNMFM69N4MHNAYBZ1X"

func store(t *testing.T) *Store {
	t.Helper()
	return StoreBeside(filepath.Join(t.TempDir(), "evomem.db"))
}

func TestSaveAndOpen(t *testing.T) {
	s := store(t)

	name, err := s.Save(noteID, "audio/ogg", strings.NewReader("OggS-pretend"))
	if err != nil {
		t.Fatal(err)
	}
	// Beside the database, named by the note, with the extension the
	// content type implies.
	if filepath.Base(name) != noteID+".ogg" {
		t.Errorf("stored as %s", filepath.Base(name))
	}
	if filepath.Base(filepath.Dir(name)) != "audio" {
		t.Errorf("stored in %s", filepath.Dir(name))
	}

	f, opened, err := s.Open(noteID)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "OggS-pretend" {
		t.Errorf("read %q", body)
	}
	// The name carries the extension, which is how the transcription
	// service decides what to decode.
	if opened != noteID+".ogg" {
		t.Errorf("opened %q", opened)
	}
}

// A note identifier becomes a filename, so this is the one place a sender
// could choose a path.
func TestSaveRefusesAnythingThatIsNotANoteID(t *testing.T) {
	s := store(t)
	for _, id := range []string{
		"../../etc/passwd",
		"01M4D3H3HNMFM69N4MHNAYBZ1X/../../x",
		"",
		"not-a-ulid",
		strings.Repeat("A", 40),
	} {
		if _, err := s.Save(id, "audio/ogg", strings.NewReader("x")); err == nil {
			t.Errorf("Save(%q) was accepted", id)
		}
	}
	// And nothing was written anywhere.
	if entries, err := os.ReadDir(s.Dir()); err == nil && len(entries) > 0 {
		t.Errorf("%d files were written", len(entries))
	}
}

func TestSaveRefusesAFormatNothingCanDecode(t *testing.T) {
	s := store(t)
	for _, mediaType := range []string{"text/plain", "application/octet-stream", "", "audio/flac"} {
		if _, err := s.Save(noteID, mediaType, strings.NewReader("x")); err == nil {
			t.Errorf("Save with %q was accepted", mediaType)
		}
	}
}

func TestExtensionForIgnoresParametersAndCase(t *testing.T) {
	for _, mediaType := range []string{"audio/ogg; codecs=opus", "AUDIO/OGG", " audio/ogg "} {
		if ext, ok := ExtensionFor(mediaType); !ok || ext != ".ogg" {
			t.Errorf("ExtensionFor(%q) = %q, %v", mediaType, ext, ok)
		}
	}
}

func TestSaveRefusesWhatIsTooLargeOrEmpty(t *testing.T) {
	s := store(t)

	if _, err := s.Save(noteID, "audio/ogg", strings.NewReader("")); err == nil {
		t.Error("an empty recording was accepted")
	}
	big := strings.NewReader(strings.Repeat("a", MaxBytes+1))
	if _, err := s.Save(noteID, "audio/ogg", big); err == nil {
		t.Error("a recording over the bound was accepted")
	}
	// Neither left anything behind, including the temporary file.
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("left behind %v", names)
	}
}

// A re-upload in another format must not leave two recordings of one note
// with only one of them reachable.
func TestSaveReplacesAnEarlierFormat(t *testing.T) {
	s := store(t)
	if _, err := s.Save(noteID, "audio/ogg", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(noteID, "audio/wav", strings.NewReader("second")); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d files, want 1", len(entries))
	}
	if entries[0].Name() != noteID+".wav" {
		t.Errorf("kept %s", entries[0].Name())
	}
}

func TestOpenSaysWhenThereIsNoRecording(t *testing.T) {
	s := store(t)
	_, _, err := s.Open(noteID)
	if !errors.Is(err, ErrNoRecording) {
		t.Errorf("err = %v, want ErrNoRecording", err)
	}
}

func TestRemoveTakesTheRecordingWhateverFormat(t *testing.T) {
	s := store(t)
	if _, err := s.Save(noteID, "audio/m4a", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(noteID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open(noteID); !errors.Is(err, ErrNoRecording) {
		t.Errorf("the recording is still there: %v", err)
	}
}

// Remove is called on every delete, and most notes are text.
func TestRemoveIsFineWithNothingThere(t *testing.T) {
	s := store(t)
	if err := s.Remove(noteID); err != nil {
		t.Errorf("Remove with no recording: %v", err)
	}
	if err := s.RemoveAll(); err != nil {
		t.Errorf("RemoveAll with no directory: %v", err)
	}
}

func TestRemoveRefusesAnythingThatIsNotANoteID(t *testing.T) {
	s := store(t)
	if err := s.Remove("../../etc"); err == nil {
		t.Error("Remove with a path was accepted")
	}
}

// A store nobody uses leaves nothing behind.
func TestTheDirectoryIsNotMadeUntilSomethingIsSaved(t *testing.T) {
	s := store(t)
	if _, err := os.Stat(s.Dir()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the directory exists before anything was saved: %v", err)
	}
}

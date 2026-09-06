package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func recordPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "proxy.pid")
}

// The record survives the round trip it exists for: one proxy writes what it
// bound, another process reads it back.
func TestARecordSurvivesTheRoundTrip(t *testing.T) {
	path := recordPath(t)
	want := Record{PID: 4321, Version: "1.2.3", Port: "51234"}
	if err := WriteRecord(path, want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadRecord(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != want {
		t.Errorf("read %+v, want %+v", got, want)
	}
}

// A build with no version stamp still writes a parseable record, and the port
// stays in field three. Dropping the placeholder would shift it into field two,
// where a reader takes it for a version.
func TestAnUnstampedBuildKeepsThePortInPlace(t *testing.T) {
	path := recordPath(t)
	if err := WriteRecord(path, Record{PID: 7, Port: "6564"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the test owns this path
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if string(raw) != "7 - 6564" {
		t.Errorf("wrote %q, want the version placeholder to hold field two", raw)
	}
	got, err := ReadRecord(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Version != "" || got.Port != "6564" {
		t.Errorf("read %+v, want an empty version and the port", got)
	}
}

// A record from an older proxy has fewer fields. What can still be learned from
// it is returned rather than refused: a reader that wanted the PID should get
// it even when there is no port to have.
func TestAShorterRecordYieldsWhatItHas(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       Record
	}{
		{"pid only", "99", Record{PID: 99}},
		{"pid and version", "99 1.0.0", Record{PID: 99, Version: "1.0.0"}},
		{"trailing newline", "99 1.0.0 6563\n", Record{PID: 99, Version: "1.0.0", Port: "6563"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := recordPath(t)
			if err := os.WriteFile(path, []byte(tc.body), FilePermRW); err != nil {
				t.Fatal(err)
			}
			got, err := ReadRecord(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if got != tc.want {
				t.Errorf("read %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A record that names no usable process is refused outright, because the two
// ways it can be wrong are the two ways a reader gets hurt: a file with no PID
// to check, and a PID that cannot be one.
func TestARecordWithoutAUsablePIDIsRefused(t *testing.T) {
	for _, body := range []string{"", "   ", "not-a-pid", "0", "-3 1.0.0 6563"} {
		path := recordPath(t)
		if err := os.WriteFile(path, []byte(body), FilePermRW); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadRecord(path); err == nil {
			t.Errorf("ReadRecord(%q) = nil error, want a refusal", body)
		}
	}
}

// A record outlives the proxy that wrote it, so liveness is what makes it
// trustworthy. Without this a reader connects to a port nothing is listening
// on, or to whatever recycled the PID.
func TestLiveRecordRefusesADeadOne(t *testing.T) {
	path := recordPath(t)
	if err := WriteRecord(path, Record{PID: os.Getpid(), Version: "1.0.0", Port: "6564"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Both answers are driven rather than one of them resting on what the host
	// says about this process, so the claim is about LiveRecord and holds on
	// every platform.
	restore := IsPIDAlive
	t.Cleanup(func() { IsPIDAlive = restore })

	IsPIDAlive = func(pid int) bool { return pid == os.Getpid() }
	if got, ok := LiveRecord(path); !ok || got.Port != "6564" {
		t.Fatalf("LiveRecord = %+v, %v; want the record whose process is alive", got, ok)
	}

	IsPIDAlive = func(int) bool { return false }
	if _, ok := LiveRecord(path); ok {
		t.Error("a record whose process is gone was reported live")
	}
}

// And a missing file is not live, rather than an error the caller has to
// distinguish: no proxy has published anything, which is an ordinary state.
func TestLiveRecordIsQuietWhenNothingHasPublished(t *testing.T) {
	if _, ok := LiveRecord(recordPath(t)); ok {
		t.Error("reported a live record where no file exists")
	}
}

// The writer refuses what the reader would, so a bad record is an error to
// whoever wrote it rather than a file every reader rejects and nobody owns.
//
// Whitespace is the subtle half: the fields are space separated, so a space
// inside one shifts every field after it and a reader takes the tail of a
// version for the port.
func TestWriteRecordRefusesWhatReadRecordWould(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  Record
	}{
		{"no pid", Record{Port: "6564"}},
		{"negative pid", Record{PID: -1, Port: "6564"}},
		{"space in version", Record{PID: 1, Version: "1.0 rc1", Port: "6564"}},
		{"space in port", Record{PID: 1, Version: "1.0", Port: "65 64"}},
		{"newline in port", Record{PID: 1, Version: "1.0", Port: "6564\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := recordPath(t)
			if err := WriteRecord(path, tc.rec); err == nil {
				t.Fatal("wrote a record no reader could use")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("a refused record left a file behind: %v", err)
			}
		})
	}
}

// A reader never sees a half-written record. A truncating write has a window
// in which the file is empty, which reads as "no proxy is running" — and
// nothing holds a lock against it: neither writer takes one, and neither will
// a reader.
func TestARecordIsNeverReadHalfWritten(t *testing.T) {
	path := recordPath(t)
	if err := WriteRecord(path, Record{PID: 1, Version: "1.0.0", Port: "6563"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if err := WriteRecord(path, Record{PID: 2, Version: "2.0.0", Port: "6564"}); err != nil {
				t.Errorf("write: %v", err)
				return
			}
		}
	}()

	for i := 0; i < 400; i++ {
		got, err := ReadRecord(path)
		if err != nil {
			t.Fatalf("read saw a partial record: %v", err)
		}
		if got.Port != "6563" && got.Port != "6564" {
			t.Fatalf("read a record that was never written whole: %+v", got)
		}
	}
	<-done
}

// The temp file the atomic write uses does not survive as a stray record.
func TestAnAtomicWriteLeavesNoDebris(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy.pid")
	if err := WriteRecord(path, Record{PID: 1, Port: "6564"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "proxy.pid" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want just the record", names)
	}
}

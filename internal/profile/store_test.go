package profile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "profiles.json")
	store := NewStore(path)
	want := Profile{APIURL: "https://baize.example.com/api/v1"}
	if err := store.Put("prod", want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get("prod")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o600 {
			t.Fatalf("config mode = %o, want 600", gotMode)
		}
	}
}

func TestStoreRejectsInvalidProfileName(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err := store.Put("../prod", Profile{}); err == nil {
		t.Fatal("Put() accepted an invalid profile name")
	}
}

func TestStoreReturnsNotFound(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "profiles.json"))
	_, err := store.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

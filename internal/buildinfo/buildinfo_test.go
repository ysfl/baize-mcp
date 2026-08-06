package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyExecutableAcceptsMatchingChecksum(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "baize-mcp")
	writeFile(t, executable, []byte("release binary"))
	writeChecksum(t, directory, executable)

	if err := verifyExecutable(executable, filepath.Join(directory, checksumFileName), true); err != nil {
		t.Fatalf("verifyExecutable() error = %v", err)
	}
}

func TestVerifyExecutableRejectsChangedChecksum(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "baize-mcp")
	writeFile(t, executable, []byte("release binary"))
	writeChecksum(t, directory, executable)
	writeFile(t, executable, []byte("changed binary"))

	if err := verifyExecutable(executable, filepath.Join(directory, checksumFileName), true); err == nil {
		t.Fatal("verifyExecutable() accepted a changed executable")
	}
}

func TestVerifyExecutableAllowsMissingChecksumForSourceBuild(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "baize-mcp")
	writeFile(t, executable, []byte("source binary"))

	if err := verifyExecutable(executable, filepath.Join(directory, checksumFileName), false); err != nil {
		t.Fatalf("verifyExecutable() error = %v", err)
	}
}

func TestVerifyExecutableRejectsMissingChecksumForReleaseBuild(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "baize-mcp")
	writeFile(t, executable, []byte("release binary"))

	if err := verifyExecutable(executable, filepath.Join(directory, checksumFileName), true); err == nil {
		t.Fatal("verifyExecutable() accepted missing release metadata")
	}
}

func TestParseChecksumRejectsUnexpectedFileName(t *testing.T) {
	checksum := sha256.Sum256([]byte("release binary"))
	metadata := []byte(hex.EncodeToString(checksum[:]) + "  other-file")

	if _, err := parseChecksum(metadata, "baize-mcp"); err == nil {
		t.Fatal("parseChecksum() accepted an unexpected file name")
	}
}

func writeChecksum(t *testing.T, directory, executable string) {
	t.Helper()
	content, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	checksum := sha256.Sum256(content)
	metadata := hex.EncodeToString(checksum[:]) + "  " + filepath.Base(executable) + "\n"
	writeFile(t, filepath.Join(directory, checksumFileName), []byte(metadata))
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

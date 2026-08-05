package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	renameio "github.com/google/renameio/v2/maybe"
)

const schemaVersion = 1

var (
	ErrNotFound   = errors.New("profile not found")
	profileNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type Profile struct {
	APIURL    string `json:"apiUrl"`
	AllowHTTP bool   `json:"allowHttp,omitempty"`
}

type fileData struct {
	Version  int                `json:"version"`
	Profiles map[string]Profile `json:"profiles"`
}

type Store struct {
	path string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return filepath.Join(dir, "baize-mcp", "profiles.json"), nil
}

func NewDefaultStore() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return NewStore(path), nil
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func ValidateName(name string) error {
	if name != strings.TrimSpace(name) || !profileNameRE.MatchString(name) {
		return errors.New("profile name must use 1-64 letters, numbers, dots, underscores, or hyphens")
	}
	return nil
}

func (s *Store) Get(name string) (Profile, error) {
	if err := ValidateName(name); err != nil {
		return Profile{}, err
	}
	data, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	item, ok := data.Profiles[name]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return item, nil
}

func (s *Store) Put(name string, item Profile) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	data, err := s.load()
	if err != nil {
		return err
	}
	data.Profiles[name] = item
	return s.save(data)
}

func (s *Store) load() (fileData, error) {
	data := fileData{Version: schemaVersion, Profiles: map[string]Profile{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return fileData{}, fmt.Errorf("read profile configuration: %w", err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fileData{}, errors.New("profile configuration is not valid JSON")
	}
	if data.Version != schemaVersion {
		return fileData{}, fmt.Errorf("unsupported profile configuration version: %d", data.Version)
	}
	if data.Profiles == nil {
		data.Profiles = map[string]Profile{}
	}
	return data, nil
}

func (s *Store) save(data fileData) error {
	data.Version = schemaVersion
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile configuration: %w", err)
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create profile configuration directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect profile configuration directory: %w", err)
	}
	if err := renameio.WriteFile(s.path, raw, 0o600); err != nil {
		return fmt.Errorf("write profile configuration: %w", err)
	}
	return nil
}

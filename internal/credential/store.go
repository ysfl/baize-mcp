package credential

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const serviceName = "baize-mcp"

var ErrNotFound = errors.New("credential not found")

type Store interface {
	Get(profile string) (string, error)
	Set(profile, token string) error
	Delete(profile string) error
}

type KeyringStore struct{}

func NewKeyringStore() *KeyringStore {
	return &KeyringStore{}
}

func (s *KeyringStore) Get(profile string) (string, error) {
	value, err := keyring.Get(serviceName, accountName(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read operating-system credential store: %w", err)
	}
	return value, nil
}

func (s *KeyringStore) Set(profile, token string) error {
	if token == "" {
		return errors.New("refusing to store an empty session credential")
	}
	if err := keyring.Set(serviceName, accountName(profile), token); err != nil {
		return fmt.Errorf("write operating-system credential store: %w", err)
	}
	return nil
}

func (s *KeyringStore) Delete(profile string) error {
	err := keyring.Delete(serviceName, accountName(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete operating-system credential: %w", err)
	}
	return nil
}

func accountName(profile string) string {
	return "session:" + profile
}

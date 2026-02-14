package secrets

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

type Store interface {
	Set(pkg, key, value string) error
	Get(pkg, key string) (string, error)
	Delete(pkg, key string) error
}

type KeyringStore struct {
	servicePrefix string
}

func NewKeyringStore() *KeyringStore {
	return &KeyringStore{servicePrefix: "mcper"}
}

func (s *KeyringStore) Set(pkg, key, value string) error {
	service := s.service(pkg)
	if err := keyring.Set(service, key, value); err != nil {
		return fmt.Errorf("set secret %s/%s: %w", pkg, key, err)
	}
	return nil
}

func (s *KeyringStore) Get(pkg, key string) (string, error) {
	service := s.service(pkg)
	v, err := keyring.Get(service, key)
	if err != nil {
		return "", err
	}
	return v, nil
}

func (s *KeyringStore) Delete(pkg, key string) error {
	service := s.service(pkg)
	if err := keyring.Delete(service, key); err != nil {
		return fmt.Errorf("delete secret %s/%s: %w", pkg, key, err)
	}
	return nil
}

func (s *KeyringStore) service(pkg string) string {
	return fmt.Sprintf("%s/%s", s.servicePrefix, pkg)
}

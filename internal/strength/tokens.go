package strength

import (
	"errors"
	"sync"
)

// ErrNoToken means the token store holds nothing for this install.
var ErrNoToken = errors.New("no 99-Strength token is stored")

// TokenStore keeps the personal API token. The token is never written to
// tcc.db, config files, logs, events or API responses; this is its only home.
type TokenStore interface {
	Get() (string, error) // ErrNoToken when there is none
	Set(token string) error
	Delete() error
	// Where describes the store to a person, e.g. "Windows Credential Manager".
	Where() string
}

// MemoryTokens is a TokenStore for tests: it forgets when the process ends.
type MemoryTokens struct {
	mu    sync.Mutex
	token string
}

func (m *MemoryTokens) Get() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token == "" {
		return "", ErrNoToken
	}
	return m.token, nil
}

func (m *MemoryTokens) Set(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = token
	return nil
}

func (m *MemoryTokens) Delete() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = ""
	return nil
}

func (m *MemoryTokens) Where() string { return "memory (not saved)" }

// credentialTarget names the stored credential. The home folder is part of
// it so a throwaway TCC_HOME never touches the real install's token.
func credentialTarget(home string) string { return "terran-cc:99-strength:" + home }

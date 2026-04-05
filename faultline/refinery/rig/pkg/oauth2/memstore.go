package oauth2

import (
	"fmt"
	"sync"
)

// MemStore is an in-memory Store implementation for testing and development.
type MemStore struct {
	mu       sync.RWMutex
	clients  map[string]*Client
	codes    map[string]*AuthorizationCode
	access   map[string]*AccessToken
	refresh  map[string]*RefreshToken
}

// NewMemStore creates a new in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		clients: make(map[string]*Client),
		codes:   make(map[string]*AuthorizationCode),
		access:  make(map[string]*AccessToken),
		refresh: make(map[string]*RefreshToken),
	}
}

// RegisterClient adds a client to the store.
func (m *MemStore) RegisterClient(c *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[c.ID] = c
}

func (m *MemStore) GetClient(id string) (*Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[id]
	if !ok {
		return nil, fmt.Errorf("client not found: %s", id)
	}
	return c, nil
}

func (m *MemStore) SaveAuthorizationCode(code *AuthorizationCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes[code.Code] = code
	return nil
}

func (m *MemStore) GetAuthorizationCode(code string) (*AuthorizationCode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.codes[code]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (m *MemStore) DeleteAuthorizationCode(code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.codes, code)
	return nil
}

func (m *MemStore) SaveAccessToken(token *AccessToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.access[token.Token] = token
	return nil
}

func (m *MemStore) GetAccessToken(token string) (*AccessToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.access[token]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *MemStore) RevokeAccessToken(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.access, token)
	return nil
}

func (m *MemStore) SaveRefreshToken(token *RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refresh[token.Token] = token
	return nil
}

func (m *MemStore) GetRefreshToken(token string) (*RefreshToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.refresh[token]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *MemStore) RevokeRefreshToken(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.refresh, token)
	return nil
}

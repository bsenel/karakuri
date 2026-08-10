package auth

import (
	"context"
	"fmt"
	"time"
)

// MemoryStore also implements CredentialStore, so the reference implementation
// covers the whole surface a consumer needs to run.
var _ CredentialStore = (*MemoryStore)(nil)

func (s *MemoryStore) GetCredential(_ context.Context, principalID string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.credentials[principalID]
	if !ok {
		return Credential{}, fmt.Errorf("%w: %q", ErrCredentialNotFound, principalID)
	}
	return c, nil
}

func (s *MemoryStore) PutCredential(_ context.Context, c Credential) error {
	if c.PrincipalID == "" {
		return fmt.Errorf("%w: credential needs a principal ID", ErrInvalidPattern)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentials[c.PrincipalID] = c
	return nil
}

func (s *MemoryStore) DeleteCredential(_ context.Context, principalID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.credentials[principalID]; !ok {
		return fmt.Errorf("%w: %q", ErrCredentialNotFound, principalID)
	}
	delete(s.credentials, principalID)
	return nil
}

func (s *MemoryStore) PutRefreshToken(_ context.Context, t RefreshToken) error {
	if t.ID == "" || t.TokenHash == "" || t.PrincipalID == "" || t.FamilyID == "" {
		return fmt.Errorf("%w: refresh token needs an ID, hash, family and principal", ErrInvalidPattern)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh[t.ID] = t
	return nil
}

func (s *MemoryStore) GetRefreshTokenByHash(_ context.Context, hash string) (RefreshToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.refresh {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return RefreshToken{}, ErrInvalidRefreshToken
}

func (s *MemoryStore) SpendRefreshToken(_ context.Context, id string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.refresh[id]
	if !ok {
		return false, ErrInvalidRefreshToken
	}
	// Compare-and-set under the write lock: concurrent exchanges of the same
	// token produce exactly one winner.
	if t.UsedAt != nil {
		return false, nil
	}
	used := at
	t.UsedAt = &used
	s.refresh[id] = t
	return true, nil
}

func (s *MemoryStore) RevokeRefreshFamily(_ context.Context, familyID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.refresh {
		if t.FamilyID == familyID && t.RevokedAt == nil {
			revoked := at
			t.RevokedAt = &revoked
			s.refresh[id] = t
		}
	}
	return nil
}

func (s *MemoryStore) RevokeRefreshTokensForPrincipal(_ context.Context, principalID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.refresh {
		if t.PrincipalID == principalID && t.RevokedAt == nil {
			revoked := at
			t.RevokedAt = &revoked
			s.refresh[id] = t
		}
	}
	return nil
}

// ListRefreshTokens returns every stored refresh token for a principal. It is a
// MemoryStore extra (not part of CredentialStore) used by tests and the example
// server to show a rotation chain.
func (s *MemoryStore) ListRefreshTokens(_ context.Context, principalID string) []RefreshToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []RefreshToken
	for _, t := range s.refresh {
		if principalID == "" || t.PrincipalID == principalID {
			out = append(out, t)
		}
	}
	return out
}

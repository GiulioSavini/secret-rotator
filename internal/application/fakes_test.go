package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/giulio/secret-rotator/internal/domain"
)

// In-memory doubles for every port, so the use case can be driven through all
// of its failure paths without a database, a Docker daemon or a filesystem.

// fakeEnvStore keeps .env files as byte slices with a single parsed key/value
// map each, which is all the use case needs from them.
type fakeEnvStore struct {
	mu       sync.Mutex
	contents map[domain.FilePath]string
	values   map[domain.FilePath]map[domain.EnvKey]string

	// failWriteOn makes Write fail for one path, to exercise compensation.
	failWriteOn domain.FilePath
	// failRestoreOn makes Restore fail for one path.
	failRestoreOn domain.FilePath

	writes   []domain.FilePath
	restores []domain.FilePath
}

func newFakeEnvStore() *fakeEnvStore {
	return &fakeEnvStore{
		contents: map[domain.FilePath]string{},
		values:   map[domain.FilePath]map[domain.EnvKey]string{},
	}
}

// put seeds a file with one key.
func (s *fakeEnvStore) put(path domain.FilePath, key domain.EnvKey, value string) {
	s.contents[path] = fmt.Sprintf("%s=%s\n", key, value)
	if s.values[path] == nil {
		s.values[path] = map[domain.EnvKey]string{}
	}
	s.values[path][key] = value
}

func (s *fakeEnvStore) Open(path domain.FilePath) (domain.EnvDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, ok := s.contents[path]
	if !ok {
		return nil, fmt.Errorf("open %s: no such file", path)
	}
	return fakeDocument{path: path, values: s.values[path], raw: []byte(content)}, nil
}

func (s *fakeEnvStore) Write(path domain.FilePath, key domain.EnvKey, value domain.Credential) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	before, ok := s.contents[path]
	if !ok {
		return nil, fmt.Errorf("write %s: no such file", path)
	}
	if path == s.failWriteOn {
		return nil, errors.New("disk full")
	}

	s.writes = append(s.writes, path)
	s.contents[path] = fmt.Sprintf("%s=%s\n", key, value.Expose())
	s.values[path][key] = value.Expose()
	return []byte(before), nil
}

func (s *fakeEnvStore) Restore(path domain.FilePath, snapshot []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if path == s.failRestoreOn {
		return errors.New("read-only filesystem")
	}
	s.restores = append(s.restores, path)
	s.contents[path] = string(snapshot)
	return nil
}

// valueOf returns the current value of a key, for assertions.
func (s *fakeEnvStore) valueOf(path domain.FilePath, key domain.EnvKey) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[path][key]
}

// contentOf returns the current raw content of a file, for assertions.
func (s *fakeEnvStore) contentOf(path domain.FilePath) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contents[path]
}

type fakeDocument struct {
	path   domain.FilePath
	values map[domain.EnvKey]string
	raw    []byte
}

func (d fakeDocument) Path() domain.FilePath { return d.path }
func (d fakeDocument) Lookup(k domain.EnvKey) (string, bool) {
	v, ok := d.values[k]
	return v, ok
}
func (d fakeDocument) Keys() []domain.EnvKey {
	keys := make([]domain.EnvKey, 0, len(d.values))
	for k := range d.values {
		keys = append(keys, k)
	}
	return keys
}
func (d fakeDocument) Snapshot() []byte { return d.raw }

// fakeRotator records what it was asked to do and can be made to fail at any
// stage.
type fakeRotator struct {
	kind domain.Kind

	applyErr   error
	verifyErr  error
	restoreErr error

	applied  []domain.Credential
	verified []domain.Credential
	restored []domain.Credential
	// seenAdminPassword records the credential the use case resolved.
	seenAdminPassword domain.Credential
}

func (r *fakeRotator) Kind() domain.Kind { return r.kind }

func (r *fakeRotator) Apply(_ context.Context, target domain.Target, next domain.Credential) error {
	r.seenAdminPassword = target.AdminPassword
	if r.applyErr != nil {
		return r.applyErr
	}
	r.applied = append(r.applied, next)
	return nil
}

func (r *fakeRotator) Verify(_ context.Context, _ domain.Target, c domain.Credential) error {
	if r.verifyErr != nil {
		return r.verifyErr
	}
	r.verified = append(r.verified, c)
	return nil
}

func (r *fakeRotator) Restore(_ context.Context, _ domain.Target, previous, _ domain.Credential) error {
	if r.restoreErr != nil {
		return r.restoreErr
	}
	r.restored = append(r.restored, previous)
	return nil
}

// fakeRegistry serves one rotator.
type fakeRegistry struct {
	rotator domain.CredentialRotator
	err     error
}

func (r fakeRegistry) For(domain.Kind) (domain.CredentialRotator, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.rotator, nil
}

// fakeGenerator returns a fixed value, so assertions can name it.
type fakeGenerator struct {
	value string
	err   error
	calls []domain.PasswordLength
}

func (g *fakeGenerator) Generate(length domain.PasswordLength) (domain.Credential, error) {
	g.calls = append(g.calls, length)
	if g.err != nil {
		return domain.Credential{}, g.err
	}
	return domain.NewCredential(g.value), nil
}

// fakeFleet records restarts and can fail either operation.
type fakeFleet struct {
	resolveTo  []domain.ContainerRef
	resolveErr error
	restartErr error

	restarts [][]domain.ContainerRef
}

func (f *fakeFleet) Resolve(_ context.Context, refs []domain.ContainerRef) ([]domain.ContainerRef, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	if f.resolveTo != nil {
		return f.resolveTo, nil
	}
	return refs, nil
}

func (f *fakeFleet) Restart(_ context.Context, refs []domain.ContainerRef) error {
	f.restarts = append(f.restarts, append([]domain.ContainerRef(nil), refs...))
	if f.restartErr != nil && len(f.restarts) == 1 {
		// Only the first restart fails; the compensating one succeeds, which
		// is the case worth asserting on.
		return f.restartErr
	}
	return nil
}

// fakeAudit records what was written to the trail.
type fakeAudit struct {
	records []domain.Record
	err     error
}

func (a *fakeAudit) Record(_ context.Context, record domain.Record) error {
	a.records = append(a.records, record)
	return a.err
}

// fixedClock makes timestamps deterministic.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// secretFor builds a valid aggregate for the tests.
func secretFor(t *testing.T, mutate func(*domain.SecretSpec)) *domain.Secret {
	t.Helper()

	spec := domain.SecretSpec{
		Name:       "db_password",
		Kind:       "postgres",
		EnvKey:     "DB_PASSWORD",
		EnvFiles:   []string{".env"},
		Containers: []string{"db", "app"},
		Target: domain.Target{
			Host:          "db",
			AdminUser:     "postgres",
			AdminPassword: domain.NewCredential("admin-pw"),
			Database:      "app",
		},
	}
	if mutate != nil {
		mutate(&spec)
	}

	secret, _, err := domain.NewSecret(spec)
	require.NoError(t, err)
	return secret
}

package keychain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hangingStore blocks until released, standing in for a Secret Service call
// that never returns because the D-Bus prompter cannot spawn.
type hangingStore struct{ release chan struct{} }

func (h *hangingStore) GetCredentials(string) (Credentials, error) {
	<-h.release
	return Credentials{Token: "too-late"}, nil
}

func (h *hangingStore) SetCredentials(string, Credentials) error {
	<-h.release
	return nil
}

func (h *hangingStore) DeleteCredentials(string) error {
	<-h.release
	return nil
}

func TestWithTimeout_GetGivesUpRatherThanHanging(t *testing.T) {
	h := &hangingStore{release: make(chan struct{})}
	defer close(h.release)

	store := withTimeout(h, 20*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := store.GetCredentials("astronomer.io")
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTimeout)
	case <-time.After(2 * time.Second):
		t.Fatal("GetCredentials hung instead of timing out")
	}
}

func TestWithTimeout_SetGivesUpRatherThanHanging(t *testing.T) {
	h := &hangingStore{release: make(chan struct{})}
	defer close(h.release)

	store := withTimeout(h, 20*time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- store.SetCredentials("astronomer.io", Credentials{Token: "t"}) }()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTimeout)
	case <-time.After(2 * time.Second):
		t.Fatal("SetCredentials hung instead of timing out")
	}
}

func TestWithTimeout_PassesThroughWhenStoreAnswers(t *testing.T) {
	inner := NewTestStore()
	store := withTimeout(inner, time.Second)

	require.NoError(t, store.SetCredentials("astronomer.io", Credentials{Token: "Bearer tok"}))

	creds, err := store.GetCredentials("astronomer.io")
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok", creds.Token)

	require.NoError(t, store.DeleteCredentials("astronomer.io"))
	_, err = store.GetCredentials("astronomer.io")
	assert.True(t, errors.Is(err, ErrNotFound))
}

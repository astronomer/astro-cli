package keychain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) GetCredentials(domain string) (Credentials, error) {
	args := m.Called(domain)
	return args.Get(0).(Credentials), args.Error(1)
}

func (m *mockStore) SetCredentials(domain string, creds Credentials) error {
	return m.Called(domain, creds).Error(0)
}

func (m *mockStore) DeleteCredentials(domain string) error {
	return m.Called(domain).Error(0)
}

func TestCachedStore_Get(t *testing.T) {
	t.Run("cache populated on first get, second get uses cache", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("GetCredentials", "example.com").Return(Credentials{Token: "tok"}, nil)
		store := newCachedStore(inner)

		got, err := store.GetCredentials("example.com")
		require.NoError(t, err)
		assert.Equal(t, "tok", got.Token)

		got, err = store.GetCredentials("example.com")
		require.NoError(t, err)
		assert.Equal(t, "tok", got.Token)

		inner.AssertNumberOfCalls(t, "GetCredentials", 1)
	})

	t.Run("different domains cached independently", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("GetCredentials", "a.io").Return(Credentials{Token: "a"}, nil)
		inner.On("GetCredentials", "b.io").Return(Credentials{Token: "b"}, nil)
		store := newCachedStore(inner)

		_, err := store.GetCredentials("a.io")
		require.NoError(t, err)

		b, err := store.GetCredentials("b.io")
		require.NoError(t, err)
		assert.Equal(t, "b", b.Token)

		inner.AssertNumberOfCalls(t, "GetCredentials", 2)
	})

	t.Run("inner error propagated and not cached", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("GetCredentials", "example.com").Return(Credentials{}, errors.New("keyring locked")).Once()
		inner.On("GetCredentials", "example.com").Return(Credentials{Token: "recovered"}, nil).Once()
		store := newCachedStore(inner)

		_, err := store.GetCredentials("example.com")
		require.ErrorContains(t, err, "keyring locked")

		got, err := store.GetCredentials("example.com")
		require.NoError(t, err)
		assert.Equal(t, "recovered", got.Token)

		inner.AssertNumberOfCalls(t, "GetCredentials", 2)
	})
}

func TestCachedStore_Set(t *testing.T) {
	t.Run("write-through then served from cache", func(t *testing.T) {
		inner := new(mockStore)
		creds := Credentials{Token: "new-tok"}
		inner.On("SetCredentials", "example.com", creds).Return(nil)
		store := newCachedStore(inner)

		require.NoError(t, store.SetCredentials("example.com", creds))

		got, err := store.GetCredentials("example.com")
		require.NoError(t, err)
		assert.Equal(t, "new-tok", got.Token)

		inner.AssertNotCalled(t, "GetCredentials")
	})

	t.Run("inner set error propagated and cache not updated", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("SetCredentials", "example.com", mock.Anything).Return(errors.New("disk full"))
		inner.On("GetCredentials", "example.com").Return(Credentials{}, ErrNotFound)
		store := newCachedStore(inner)

		err := store.SetCredentials("example.com", Credentials{Token: "tok"})
		require.ErrorContains(t, err, "disk full")

		_, err = store.GetCredentials("example.com")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestCachedStore_Delete(t *testing.T) {
	t.Run("invalidates cache so next get hits inner", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("GetCredentials", "example.com").Return(Credentials{Token: "tok"}, nil).Once()
		inner.On("GetCredentials", "example.com").Return(Credentials{}, ErrNotFound).Once()
		inner.On("DeleteCredentials", "example.com").Return(nil)
		store := newCachedStore(inner)

		_, err := store.GetCredentials("example.com")
		require.NoError(t, err)

		require.NoError(t, store.DeleteCredentials("example.com"))

		_, err = store.GetCredentials("example.com")
		assert.ErrorIs(t, err, ErrNotFound)

		inner.AssertNumberOfCalls(t, "GetCredentials", 2)
	})

	t.Run("does not affect other domains", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("GetCredentials", "a.io").Return(Credentials{Token: "a"}, nil)
		inner.On("GetCredentials", "b.io").Return(Credentials{Token: "b"}, nil)
		inner.On("DeleteCredentials", "a.io").Return(nil)
		store := newCachedStore(inner)

		_, err := store.GetCredentials("a.io")
		require.NoError(t, err)
		_, err = store.GetCredentials("b.io")
		require.NoError(t, err)

		require.NoError(t, store.DeleteCredentials("a.io"))

		got, err := store.GetCredentials("b.io")
		require.NoError(t, err)
		assert.Equal(t, "b", got.Token)

		inner.AssertNumberOfCalls(t, "GetCredentials", 2)
	})

	t.Run("inner delete error propagated and cache preserved", func(t *testing.T) {
		inner := new(mockStore)
		inner.On("GetCredentials", "example.com").Return(Credentials{Token: "tok"}, nil)
		inner.On("DeleteCredentials", "example.com").Return(errors.New("permission denied"))
		store := newCachedStore(inner)

		_, err := store.GetCredentials("example.com")
		require.NoError(t, err)

		err = store.DeleteCredentials("example.com")
		require.ErrorContains(t, err, "permission denied")

		got, err := store.GetCredentials("example.com")
		require.NoError(t, err)
		assert.Equal(t, "tok", got.Token)

		inner.AssertNumberOfCalls(t, "GetCredentials", 1)
	})
}

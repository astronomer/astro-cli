package houston

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleAPIError(t *testing.T) {
	t.Run("should return same error", func(t *testing.T) {
		err := errors.New("internal server error") //nolint:goerr113

		gotErr := handleAPIErr(err)
		assert.EqualError(t, gotErr, err.Error())
	})

	t.Run("should return query fields error", func(t *testing.T) {
		err := errors.New("error: Cannot query field \"triggererEnabled\" on AppConfig") //nolint:goerr113

		gotErr := handleAPIErr(err)
		assert.EqualError(t, gotErr, ErrFieldsNotAvailable{}.Error())
	})
}

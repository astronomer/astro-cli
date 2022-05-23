package houston

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	errServer = errors.New("internal server error")
	errQuery  = errors.New("error: Cannot query field \"triggererEnabled\" on AppConfig")
)

func TestHandleAPIError(t *testing.T) {
	t.Run("should return same error", func(t *testing.T) {
		gotErr := handleAPIErr(errServer)
		assert.EqualError(t, gotErr, errServer.Error())
	})

	t.Run("should return query fields error", func(t *testing.T) {
		gotErr := handleAPIErr(errQuery)
		assert.EqualError(t, gotErr, ErrFieldsNotAvailable{}.Error())
	})
}

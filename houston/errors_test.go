package houston

import (
	"errors"
)

var (
	errServer = errors.New("internal server error")
	errQuery  = errors.New("error: Cannot query field \"triggererEnabled\" on AppConfig")
)

func (s *Suite) TestHandleAPIError() {
	s.Run("should return same error", func() {
		gotErr := handleAPIErr(errServer)
		s.EqualError(gotErr, errServer.Error())
	})

	s.Run("should return query fields error", func() {
		gotErr := handleAPIErr(errQuery)
		s.EqualError(gotErr, ErrFieldsNotAvailable{}.Error())
	})
}

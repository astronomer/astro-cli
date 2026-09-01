package env

import (
	"errors"
	"fmt"
)

func (s *Suite) TestIsSecretsFetchingNotAllowedError() {
	testCases := []struct {
		name           string
		err            error
		expectedResult bool
	}{
		{
			"exact match",
			errors.New("showSecrets on organization with id clt4farh34gm801n753x80fo8 is not allowed"),
			true,
		},
		{
			"case insensitive match",
			errors.New("ShowSecrets on ORGANIZATION with id someId IS NOT ALLOWED"),
			true,
		},
		{
			"wrapped error with secrets error",
			fmt.Errorf("API error: %w", errors.New("showSecrets on organization with id test is not allowed")),
			true,
		},
		{
			"different organization error",
			errors.New("organization with id test not found"),
			false,
		},
		{
			"different secrets error",
			errors.New("secrets access denied"),
			false,
		},
		{
			"regular error",
			errors.New("network timeout"),
			false,
		},
		{
			"nil error",
			nil,
			false,
		},
		{
			"empty error",
			errors.New(""),
			false,
		},
		{
			"partial match - missing showSecrets",
			errors.New("organization with id test is not allowed"),
			false,
		},
		{
			"partial match - missing organization",
			errors.New("showSecrets with id test is not allowed"),
			false,
		},
		{
			"partial match - missing not allowed",
			errors.New("showSecrets on organization with id test is allowed"),
			false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := IsSecretsFetchingNotAllowedError(tc.err)
			s.Equal(tc.expectedResult, result, "Expected IsSecretsFetchingNotAllowedError(%v) to be %v", tc.err, tc.expectedResult)
		})
	}
}

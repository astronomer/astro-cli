package testutil

import "errors"

var errMock = errors.New("mock error")

type PromptSelectNoMock struct{}

func (p PromptSelectNoMock) Run() (index int, item string, err error) {
	return 1, "n", nil
}

type PromptSelectYesMock struct{}

func (p PromptSelectYesMock) Run() (index int, item string, err error) {
	return 0, "y", nil
}

type PromptSelectErrMock struct{}

func (p PromptSelectErrMock) Run() (index int, item string, err error) {
	return -1, "invalid item", errMock
}

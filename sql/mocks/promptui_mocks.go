package mocks

type PromptSelectNoMock struct{}

func (p PromptSelectNoMock) Run() (index int, item string, err error) {
	return 1, "n", nil
}

type PromptSelectYesMock struct{}

func (p PromptSelectYesMock) Run() (index int, item string, err error) {
	return 0, "y", nil
}

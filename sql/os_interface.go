package sql

import (
	"os"
)

type OsBinder struct{}

type OsBind interface {
	Exit(code int)
	WriteFile(name string, data []byte, perm os.FileMode) error
}

func (OsBinder) Exit(code int) {
	os.Exit(code)
}

func (OsBinder) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func NewOsBind() OsBind {
	return &OsBinder{}
}

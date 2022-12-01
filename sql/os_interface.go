package sql

import (
	"os"
)

type OsBinder struct{}

type OsBind interface {
	WriteFile(name string, data []byte, perm os.FileMode) error
}

func (OsBinder) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func NewOsBind() OsBind {
	return &OsBinder{}
}

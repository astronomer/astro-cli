package sql

import (
	"os"
)

type OsBinder struct{}

type OsBind interface {
	WriteFile(name string, data []byte, perm os.FileMode) error
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	ReadFile(name string) ([]byte, error)
}

func (OsBinder) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (OsBinder) Open(name string) (*os.File, error) {
	return os.Open(name)
}

func (OsBinder) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (OsBinder) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func NewOsBind() OsBind {
	return &OsBinder{}
}

package sql

import (
	"io"
)

type IoBinder struct {
}

type IoBind interface {
	Copy(dst io.Writer, src io.Reader) (written int64, err error)
}

func (IoBinder) Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	return io.Copy(dst, src)
}

func NewIoBind() IoBind {
	return &IoBinder{}
}

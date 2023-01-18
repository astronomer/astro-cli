package sql

import (
	"bufio"
	"io"
)

type BufIOBinder struct{}

type BufIOBind interface {
	NewScanner(r io.Reader) *bufio.Scanner
}

func (BufIOBinder) NewScanner(r io.Reader) *bufio.Scanner {
	return bufio.NewScanner(r)
}

func NewBufIOBind() BufIOBind {
	return &BufIOBinder{}
}

package sql

import (
	"bufio"
	"io"
)

type BufIoBinder struct{}

type BufIoBind interface {
	NewScanner(r io.Reader) *bufio.Scanner
}

func (BufIoBinder) NewScanner(r io.Reader) *bufio.Scanner {
	return bufio.NewScanner(r)
}

func NewBufIoBind() BufIoBind {
	return &BufIoBinder{}
}

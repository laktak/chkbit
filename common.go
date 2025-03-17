package chkbit

import (
	"errors"
)

var (
	errNotSupported = errors.New("operation not supported")
)

func IsNotSupported(err error) bool {
	return err == errNotSupported
}

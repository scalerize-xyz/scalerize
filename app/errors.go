package app

import "errors"

var (
	ErrDataNotFound         = errors.New("data not found")
	ErrReadingFromReth      = errors.New("failed to read request from reth db client")
	ErrWritingToReth        = errors.New("failed to write response to reth db client")
	ErrInvalidOperationCode = errors.New("invalid op-code, available op-codes are GET, PUT, DELETE, WRITE")
	ErrStoreNotFound        = errors.New("store not found")
)

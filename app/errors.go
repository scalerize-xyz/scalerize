package app

import "errors"

var (
	ErrDataNotFound                                     = errors.New("data not found")
	ErrReadingFromReth                                  = errors.New("failed to read request from reth db client")
	ErrWritingToReth                                    = errors.New("failed to write response to reth db client")
	ErrInvalidOperationCode                             = errors.New("invalid op-code, available op-codes are GET, PUT, DELETE, WRITE")
	ErrStoreNotFound                                    = errors.New("store not found")
	ErrStoreIsEmpty                                     = errors.New("store is empty")
	ErrKeyNotExists                                     = errors.New("exact key not exists")
	ErrExactOrGreaterKeyNotExists                       = errors.New("exact or greater key does not exists")
	ErrCurrentIteratorKeyIsInvalid                      = errors.New("current iterator key not exists")
	ErrCannotIteratePrevOrNextWhenCurrentKeyIsOnlyEntry = errors.New("cannot iterate to next or prev key, since current key is the only entry in the table")
	ErrCurrentKeyIsNotSet                               = errors.New("current key is not set")
)

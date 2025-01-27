package app

import "errors"

var (
	ErrInvalidRequestData                             = errors.New("invalid request data")
	ErrDataNotFound                                   = errors.New("data not found")
	ErrInvalidOperationCode                           = errors.New("invalid operation code")
	ErrTableNotFound                                  = errors.New("table not found")
	ErrTableIsEmpty                                   = errors.New("table is empty")
	ErrKeyNotExists                                   = errors.New("exact key not exists")
	ErrExactOrGreaterKeyNotExists                     = errors.New("exact or greater key does not exists")
	ErrCurrentIteratorKeyIsInvalid                    = errors.New("current iterator key not exists")
	ErrCannotIterateToPrevFromFirst                   = errors.New("cannot iterate to prev from first key")
	ErrCannotIterateToNextFromLast                    = errors.New("cannot iterate to next from last key")
	ErrCurrentKeyIsNotSet                             = errors.New("current key is not set")
	ErrKeyAlreadyPresent                              = errors.New("key already present")
	ErrCannotAppendIfKeyIsLessThanCurrrentGreatestKey = errors.New("cannot append entry when key is less than greatest key in the table")
)

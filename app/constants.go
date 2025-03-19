package app

import "sync"

var (
	ethIteratorsCurrentKey     = make(map[[CursorIDBytes]byte][]byte)
	ethIteratorsCurrentKeyLock sync.RWMutex
)

const (
	// DbTx and DbTxMut for both regular and dup-sorted tables
	OP_PUT    byte = 1
	OP_GET    byte = 2
	OP_DELETE byte = 3
	OP_WRITE  byte = 4

	// DbCursorRO for both regular and dup-sorted tables
	OP_FIRST      byte = 5
	OP_SEEK_EXACT byte = 6
	OP_SEEK       byte = 7
	OP_NEXT       byte = 8
	OP_PREV       byte = 9
	OP_LAST       byte = 10
	OP_CURRENT    byte = 11

	// DbCursorRW for both regular and dup-sorted tables
	OP_UPSERT         byte = 12
	OP_INSERT         byte = 13
	OP_APPEND         byte = 14
	OP_DELETE_CURRENT byte = 15

	// DbDupCursorRO for dup-sorted tables
	OP_NEXT_DUP           byte = 16
	OP_NEXT_NO_DUP        byte = 17
	OP_NEXT_DUP_VAL       byte = 18
	OP_SEEK_BY_KEY_SUBKEY byte = 19

	// DbDupCursorRW for dup-sorted tables
	OP_DELETE_CURRENT_DUPLICATES byte = 20
	OP_APPEND_DUP                byte = 21

	OP_DROP_CURSOR byte = 16
)

// STATE OPERATIONS
const (
	OP_STATE_ROOT  byte = 1
	OP_STATE_PROOF byte = 2
)

const (
	HashedAccountsStoreName = "hashed_accounts"
	HashedStoragesStoreName = "hashed_storages"

	HashedAccountsTableCode = 0
	HashedStoragesTableCode = 1

	SerializedHashedAccountsKeyBytes    = 40
	SerializedHashedStoragesKeyBytes    = 40
	SerializedHashedStoragesSubKeyBytes = 40

	EthAccountAddressBytes = 20
	EthBlockNumberBytes    = 8
	EthBlockHashBytes      = 32

	CursorIDBytes = 8
)

const (
	STATUS_SUCCESS byte = 1
	STATUS_ERROR   byte = 0
)

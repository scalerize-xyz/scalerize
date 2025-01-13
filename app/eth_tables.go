package app

// Scalerize will handle data for 2 tables from Ethereum(reth)
// 1. HashedAccounts
// 2. HashedStorages
// const NumberOfTables uint8 = 3

// need to mangage the key for:
// 1. DbCursorRO and 2. DbCursorRW for normal tables
// 3.  DbDupCursorRO and 4. DbDupCursorRW for dup sorted tables

var (
	// key for storing account data will always be last
	HashedAccountDataKey = [32]byte{
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
	}
)

const (
	HashedAccountsTableCode = 0
	HashedStoragesTableCode = 1

	HashedAccountsKeyBytes    = 32
	HashedStoragesKeyBytes    = 32
	HashedStoragesSubKeyBytes = 32
)

type TableInfo struct {
	DupSorted   bool
	KeyBytes    int
	SubKeyBytes int
	StaticKey   []byte
}

// var executionTableInfo = map[uint8]TableInfo{
// 	HashedAccountsTableCode: {
// 		DupSorted: false,
// 		KeyBytes:  32,
// 		StaticKey: []byte("account_data"),
// 	},
// 	HashedStoragesTableCode: {
// 		DupSorted:   true,
// 		KeyBytes:    32,
// 		SubKeyBytes: 32,
// 	},
// }

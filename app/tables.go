package app

import storetypes "cosmossdk.io/store/types"

type tableInfo struct {
	DupSorted   bool
	KeyBytes    int
	SubKeyBytes int
	StoreKey    storetypes.StoreKey
}

func (app *ScalerizeApp) getTable(tableCode uint8) (*tableInfo, error) {
	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	return &table, nil
}

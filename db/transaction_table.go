package db

import "errors"

var ErrForeignTransactionTable = errors.New("table belongs to another database")

// Insert adds a row and all of its secondary-index entries to the transaction.
func (tx *Tx) Insert(table *Table, row map[string]any) error {
	return tx.mutateTable(table, func() error {
		key, value, err := encodeRow(table.schema, row)
		if err != nil {
			return err
		}
		if _, found, err := tx.valueLocked(key); err != nil {
			return err
		} else if found {
			return ErrDuplicatePrimary
		}
		entries, err := transactionIndexEntries(tx, table, row, key, false)
		if err != nil {
			return err
		}
		tx.recordLocked(key, transactionChange{value: value})
		for _, entry := range entries {
			tx.recordLocked(entry.key, transactionChange{value: entry.value})
		}
		return nil
	})
}

// Update replaces a row and updates all of its secondary-index entries in the
// same transaction.
func (tx *Tx) Update(table *Table, row map[string]any) error {
	return tx.mutateTable(table, func() error {
		key, value, err := encodeRow(table.schema, row)
		if err != nil {
			return err
		}
		oldValue, found, err := tx.valueLocked(key)
		if err != nil {
			return err
		}
		if !found {
			return ErrRowNotFound
		}
		oldRow, err := decodeRow(table.schema, []byte(oldValue))
		if err != nil {
			return err
		}
		oldEntries, err := transactionIndexEntries(tx, table, oldRow, key, true)
		if err != nil {
			return err
		}
		newEntries, err := transactionIndexEntries(tx, table, row, key, true)
		if err != nil {
			return err
		}
		for _, entry := range oldEntries {
			tx.recordLocked(entry.key, transactionChange{delete: true})
		}
		tx.recordLocked(key, transactionChange{value: value})
		for _, entry := range newEntries {
			tx.recordLocked(entry.key, transactionChange{value: entry.value})
		}
		return nil
	})
}

// DeleteRow removes a row and all of its secondary-index entries in the same
// transaction.
func (tx *Tx) DeleteRow(table *Table, primaryKey any) (bool, error) {
	var deleted bool
	err := tx.mutateTable(table, func() error {
		key, err := table.rowKey(primaryKey)
		if err != nil {
			return err
		}
		value, found, err := tx.valueLocked(key)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		row, err := decodeRow(table.schema, []byte(value))
		if err != nil {
			return err
		}
		entries, err := transactionIndexEntries(tx, table, row, key, true)
		if err != nil {
			return err
		}
		tx.recordLocked(key, transactionChange{delete: true})
		for _, entry := range entries {
			tx.recordLocked(entry.key, transactionChange{delete: true})
		}
		deleted = true
		return nil
	})
	return deleted, err
}

func (tx *Tx) mutateTable(table *Table, mutation func() error) error {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	if tx.closed {
		return ErrTransactionClosed
	}
	if tx.readOnly {
		return ErrReadOnlyTransaction
	}
	if table == nil || table.db != tx.db {
		return ErrForeignTransactionTable
	}
	return mutation()
}

func transactionIndexEntries(tx *Tx, table *Table, row map[string]any, rowKey string, allowExisting bool) ([]indexEntry, error) {
	entries := make([]indexEntry, 0, len(table.indexes))
	primaryKey := []byte(rowKey[len(rowPrefix(table.schema.Name)):])
	for _, index := range table.indexes {
		column := columnByName(table.schema, index.Column)
		indexedValue, err := encodeValue(column, row[index.Column])
		if err != nil {
			return nil, err
		}
		key := indexEntryKey(table.schema.Name, index, indexedValue, primaryKey)
		if index.Unique {
			existing, found, err := tx.valueLocked(key)
			if err != nil {
				return nil, err
			}
			if found && (!allowExisting || existing != string(primaryKey)) {
				return nil, ErrDuplicateIndexed
			}
		}
		entries = append(entries, indexEntry{key: key, value: string(primaryKey)})
	}
	return entries, nil
}

package kvlog

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestLogReplaysSetRecordsAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")

	log, data, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("initial data length = %d, want 0", len(data))
	}
	if err := log.Set("a", "one"); err != nil {
		t.Fatalf("Set(a) error = %v", err)
	}
	if err := log.Set("b", "two"); err != nil {
		t.Fatalf("Set(b) error = %v", err)
	}
	if err := log.Set("a", "updated"); err != nil {
		t.Fatalf("Set(a update) error = %v", err)
	}
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, data, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer closeLog(t, reopened)

	assertValue(t, data, "a", "updated")
	assertValue(t, data, "b", "two")
}

func TestLogReplaysDeleteRecordsAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")

	log, _, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := log.Set("a", "one"); err != nil {
		t.Fatalf("Set(a) error = %v", err)
	}
	if err := log.Set("b", "two"); err != nil {
		t.Fatalf("Set(b) error = %v", err)
	}
	if err := log.Delete("a"); err != nil {
		t.Fatalf("Delete(a) error = %v", err)
	}
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, data, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer closeLog(t, reopened)

	if _, ok := data["a"]; ok {
		t.Fatal("deleted key a survived restart")
	}
	assertValue(t, data, "b", "two")
}

func TestLogIgnoresPartialTrailingRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")

	log, _, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := log.Set("a", "one"); err != nil {
		t.Fatalf("Set(a) error = %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	partial, err := encodeRecord(recordSet, "b", "two")
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}
	if _, err := file.Write(partial[:len(partial)-2]); err != nil {
		t.Fatalf("Write(partial) error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(partial file) error = %v", err)
	}

	reopened, data, err := Open(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}

	assertValue(t, data, "a", "one")
	if _, ok := data["b"]; ok {
		t.Fatal("partial trailing key b was replayed")
	}

	if err := reopened.Set("c", "three"); err != nil {
		t.Fatalf("Set(c) error = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, data, err = Open(path)
	if err != nil {
		t.Fatalf("second reopen error = %v", err)
	}
	defer closeLog(t, reopened)

	assertValue(t, data, "a", "one")
	assertValue(t, data, "c", "three")
	if _, ok := data["b"]; ok {
		t.Fatal("partial trailing key b reappeared after append")
	}
}

func TestLogRejectsBadChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	record, err := encodeRecord(recordSet, "a", "one")
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}
	record[len(record)-1] ^= 0xff
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	log, _, err := Open(path)
	if err == nil {
		closeLog(t, log)
		t.Fatal("expected checksum error")
	}
	if !errors.Is(err, errInvalidChecksum) {
		t.Fatalf("Open() error = %v, want %v", err, errInvalidChecksum)
	}
}

func TestLogRejectsInvalidRecordLengths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	record, err := encodeRecord(recordSet, "a", "one")
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}
	binary.LittleEndian.PutUint32(record[1:5], 0)
	rewriteChecksum(record)
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	log, _, err := Open(path)
	if err == nil {
		closeLog(t, log)
		t.Fatal("expected length error")
	}
	if !errors.Is(err, errInvalidLength) {
		t.Fatalf("Open() error = %v, want %v", err, errInvalidLength)
	}
}

func TestLogRejectsInvalidDeleteRecordWithValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kv.log")
	record, err := encodeRecord(recordSet, "a", "one")
	if err != nil {
		t.Fatalf("encodeRecord() error = %v", err)
	}
	record[0] = recordDelete
	rewriteChecksum(record)
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	log, _, err := Open(path)
	if err == nil {
		closeLog(t, log)
		t.Fatal("expected invalid length error")
	}
	if !errors.Is(err, errInvalidLength) {
		t.Fatalf("Open() error = %v, want %v", err, errInvalidLength)
	}
}

func assertValue(t *testing.T, data map[string]string, key, want string) {
	t.Helper()

	got, ok := data[key]
	if !ok {
		t.Fatalf("key %q missing", key)
	}
	if got != want {
		t.Fatalf("data[%q] = %q, want %q", key, got, want)
	}
}

func closeLog(t *testing.T, log *Log) {
	t.Helper()

	if err := log.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func rewriteChecksum(record []byte) {
	binary.LittleEndian.PutUint32(record[9:13], 0)
	checksum := crc32Checksum(record)
	binary.LittleEndian.PutUint32(record[9:13], checksum)
}

func crc32Checksum(record []byte) uint32 {
	checksum := crc32.ChecksumIEEE(record[0:9])
	return crc32.Update(checksum, crc32.IEEETable, record[recordHeaderSize:])
}

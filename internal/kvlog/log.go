package kvlog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

const (
	recordHeaderSize = 13
	maxRecordPayload = 64 << 20

	recordSet    = 1
	recordDelete = 2
)

var (
	errInvalidRecordType = errors.New("invalid record type")
	errInvalidChecksum   = errors.New("invalid record checksum")
	errInvalidLength     = errors.New("invalid record length")
)

type Log struct {
	file *os.File
}

// Open opens an append-only key-value log and replays existing records.
func Open(path string) (*Log, map[string]string, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, nil, err
	}

	data, err := replay(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	return &Log{file: file}, data, nil
}

// Close closes the log file.
func (l *Log) Close() error {
	return l.file.Close()
}

// Set appends a durable Set record.
func (l *Log) Set(key, value string) error {
	return l.append(recordSet, key, value)
}

// Delete appends a durable Delete record.
func (l *Log) Delete(key string) error {
	return l.append(recordDelete, key, "")
}

// Sync flushes pending log writes to durable storage.
func (l *Log) Sync() error {
	return l.file.Sync()
}

func (l *Log) append(recordType byte, key, value string) error {
	record, err := encodeRecord(recordType, key, value)
	if err != nil {
		return err
	}

	n, err := l.file.Write(record)
	if err != nil {
		return err
	}
	if n != len(record) {
		return io.ErrShortWrite
	}
	return l.file.Sync()
}

func replay(file *os.File) (map[string]string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	data := make(map[string]string)
	for {
		recordStart, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		recordType, key, value, err := readRecord(file)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return data, nil
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				if err := file.Truncate(recordStart); err != nil {
					return nil, err
				}
				return data, nil
			}
			return nil, err
		}

		switch recordType {
		case recordSet:
			data[key] = value
		case recordDelete:
			delete(data, key)
		default:
			return nil, fmt.Errorf("%w: %d", errInvalidRecordType, recordType)
		}
	}
}

func encodeRecord(recordType byte, key, value string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("%w: key is empty", errInvalidLength)
	}
	if recordType != recordSet && recordType != recordDelete {
		return nil, fmt.Errorf("%w: %d", errInvalidRecordType, recordType)
	}
	if recordType == recordDelete && value != "" {
		return nil, fmt.Errorf("%w: delete record has value", errInvalidLength)
	}

	keyLength := len(key)
	valueLength := len(value)
	if keyLength > int(^uint32(0)) || valueLength > int(^uint32(0)) {
		return nil, errInvalidLength
	}

	record := make([]byte, recordHeaderSize+keyLength+valueLength)
	record[0] = recordType
	binary.LittleEndian.PutUint32(record[1:5], uint32(keyLength))
	binary.LittleEndian.PutUint32(record[5:9], uint32(valueLength))
	copy(record[recordHeaderSize:], key)
	copy(record[recordHeaderSize+keyLength:], value)

	checksum := crc32.ChecksumIEEE(record[0:9])
	checksum = crc32.Update(checksum, crc32.IEEETable, record[recordHeaderSize:])
	binary.LittleEndian.PutUint32(record[9:13], checksum)

	return record, nil
}

func readRecord(reader io.Reader) (byte, string, string, error) {
	header := make([]byte, recordHeaderSize)
	if _, err := io.ReadFull(reader, header); err != nil {
		return 0, "", "", err
	}

	recordType := header[0]
	keyLength := binary.LittleEndian.Uint32(header[1:5])
	valueLength := binary.LittleEndian.Uint32(header[5:9])
	wantChecksum := binary.LittleEndian.Uint32(header[9:13])
	if keyLength == 0 {
		return 0, "", "", errInvalidLength
	}
	if recordType == recordDelete && valueLength != 0 {
		return 0, "", "", errInvalidLength
	}

	payloadLength := uint64(keyLength) + uint64(valueLength)
	if payloadLength > maxRecordPayload {
		return 0, "", "", errInvalidLength
	}

	payload := make([]byte, int(payloadLength))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, "", "", err
	}

	checksum := crc32.ChecksumIEEE(header[0:9])
	checksum = crc32.Update(checksum, crc32.IEEETable, payload)
	if checksum != wantChecksum {
		return 0, "", "", errInvalidChecksum
	}

	key := string(payload[:keyLength])
	value := string(payload[keyLength:])
	return recordType, key, value, nil
}

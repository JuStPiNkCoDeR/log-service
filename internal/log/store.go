// Copyright sasha.los.0148@gmail.com
// All Rights have been taken by Mafia :)
package log

import (
	"bufio"
	"encoding/binary"
	"mafia/log/lib"
	"os"
	"strconv"
	"sync"
)

const (
	lenWidth = 8
)

var (
	enc = binary.BigEndian
)

// Works for store file
type Store struct {
	*os.File
	mu   sync.Mutex
	buf  *bufio.Writer
	size uint64
}

// Creates new store from new or existing store file.
func NewStore(f *os.File) (*Store, error) {
	file, err := os.Stat(f.Name())
	if err != nil {
		return nil, lib.Wrap(err, "Unable to get file stats")
	}

	size := uint64(file.Size())
	return &Store{
		File: f,
		size: size,
		buf:  bufio.NewWriter(f),
	}, nil
}

// Read record from store file.
//
// First it flushes the writer buffer, if it hasn't flushed yet.
//
// Returns the record stored at the given position.
func (s *Store) Read(pos uint64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.buf.Flush(); err != nil {
		return nil, lib.Wrap(err, "Unable to flush store buffer")
	}

	size := make([]byte, lenWidth)
	if _, err := s.File.ReadAt(size, int64(pos)); err != nil {
		return nil, lib.Wrap(err, "Unable to read size from store file at "+strconv.FormatUint(pos, 10))
	}

	b := make([]byte, enc.Uint64(size))
	if _, err := s.File.ReadAt(b, int64(pos+lenWidth)); err != nil {
		return nil, lib.Wrap(err, "Unable to read data from store file at "+strconv.FormatUint(pos+lenWidth, 10))
	}

	return b, nil
}

// Reads len(p) bytes into p beginning at the off offset in the store’s file.
//
// It implements io.ReaderAt on the store type.
func (s *Store) ReadAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.buf.Flush(); err != nil {
		return 0, lib.Wrap(err, "Unable to flush store buffer")
	}

	return s.File.ReadAt(p, off)
}

// Append record to store file.
//
// Writes to the buffered writer instead of directly to the file.
// It reduces the number of system call -> improve performance.
//
// Returns the number of bytes written and position where the store holds the record in its file.
func (s *Store) Append(p []byte) (n uint64, pos uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pos = s.size

	if err := binary.Write(s.buf, enc, uint64(len(p))); err != nil {
		return 0, 0, lib.Wrap(err, "Unable to write record length")
	}

	w, err := s.buf.Write(p)
	if err != nil {
		return 0, 0, lib.Wrap(err, "Unable to write record data")
	}

	w += lenWidth
	s.size += uint64(w)

	return uint64(w), pos, nil
}

// Copyright sasha.los.0148@gmail.com
// All Rights have been taken by Mafia :)
package log

import (
	"github.com/gogo/protobuf/proto"
	api "mafia/log/api/v1"
	"mafia/log/lib"

	"fmt"
	"os"
	"path"
)

// Wraps Store and Index types to coordinate operations across them.
type Segment struct {
	store                  *Store
	index                  *Index
	baseOffset, nextOffset uint64
	config                 Config
}

// Initialize Store and Index object to create new Segment object.
func NewSegment(dir string, baseOffset uint64, c Config) (s *Segment, err error) {
	s = &Segment{
		baseOffset: baseOffset,
		config:     c,
	}

	storeFile, err := os.OpenFile(
		path.Join(dir, fmt.Sprintf("%d%s", baseOffset, ".store")),
		os.O_RDWR|os.O_CREATE|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, lib.Wrap(err, "Unable to open/create the store file")
	}

	if s.store, err = NewStore(storeFile); err != nil {
		return nil, lib.Wrap(err, "Unable to create Store object")
	}

	indexFile, err := os.OpenFile(
		path.Join(dir, fmt.Sprintf("%d%s", baseOffset, ".index")),
		os.O_RDWR|os.O_CREATE,
		0644,
	)
	if err != nil {
		return nil, lib.Wrap(err, "Unable to open/create the index file")
	}

	if s.index, err = NewIndex(indexFile, c); err != nil {
		return nil, lib.Wrap(err, "Unable to create Index object")
	}

	if off, _, err := s.index.Read(-1); err != nil {
		s.nextOffset = baseOffset
	} else {
		s.nextOffset = baseOffset + uint64(off) + 1
	}

	return
}

// Writes the record and returns the newly appended record's offset.
func (s *Segment) Append(record *api.Record) (offset uint64, err error) {
	offset = s.nextOffset
	record.Offset = offset

	p, err := proto.Marshal(record)
	if err != nil {
		return 0, lib.Wrap(err, "Unable to Marshal record(proto)")
	}

	_, pos, err := s.store.Append(p)
	if err != nil {
		return 0, lib.Wrap(err, "Unable to append record to store file")
	}

	if err = s.index.Write(
		// index offsets are relative to base offset
		uint32(s.nextOffset-s.baseOffset),
		pos,
	); err != nil {
		return 0, lib.Wrap(err, "Unable to write record's index")
	}

	s.nextOffset++

	return
}

// Returns the record for the given offset.
func (s *Segment) Read(off uint64) (*api.Record, error) {
	_, pos, err := s.index.Read(int64(off - s.baseOffset))
	if err != nil {
		return nil, lib.Wrap(err, "Unable to get record's index data")
	}

	p, err := s.store.Read(pos)
	if err != nil {
		return nil, lib.Wrap(err, "Unable to read record's data from store file")
	}

	record := &api.Record{}
	if err = proto.Unmarshal(p, record); err != nil {
		return nil, lib.Wrap(err, "Unable to Unmarshal record's data(proto)")
	}

	return record, nil
}

// Returns whether the segment has reached its max size.
func (s *Segment) IsMaxed() bool {
	return s.store.size >= s.config.Segment.MaxStoreBytes ||
		s.index.size >= s.config.Segment.MaxIndexBytes
}

// Close the Index and Store objects
func (s *Segment) Close() error {
	if err := s.index.Close(); err != nil {
		return lib.Wrap(err, "Unable to close index file")
	}

	if err := s.store.Close(); err != nil {
		return lib.Wrap(err, "Unable to close store file")
	}

	return nil
}

// Closes the segment and removes the index and store files.
func (s *Segment) Remove() error {
	if err := s.Close(); err != nil {
		return lib.Wrap(err, "Unable to close the segment")
	}

	// TODO may add archive algorithm here
	if err := os.Remove(s.index.Name()); err != nil {
		return lib.Wrap(err, "Unable to remove the index file")
	}

	if err := os.Remove(s.store.Name()); err != nil {
		return lib.Wrap(err, "Unable to remove the store file")
	}

	return nil
}

// Returns the nearest and lesser multiple of k in j
func NearestMultiple(j, k uint64) uint64 {
	if 0 <= j {
		return (j / k) * k
	}

	return ((j/k + 1) / k) * k
}

// Copyright sasha.los.0148@gmail.com
// All Rights have been taken by Mafia :)
package log

import (
	"io"
	"log/lib"
	"os"

	"github.com/tysontate/gommap"
)

var (
	offWidth uint64 = 4
	posWidth uint64 = 8
	entWidth        = offWidth + posWidth
)

// Works for Index file.
type Index struct {
	file *os.File
	mmap gommap.MMap
	size uint64
}

// Creates Index object for working with Index files.
func NewIndex(f *os.File, c Config) (*Index, error) {
	idx := &Index{
		file: f,
	}

	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, lib.Wrap(err, "Unable to get file stats")
	}

	idx.size = uint64(fi.Size())
	if err = os.Truncate(
		f.Name(), int64(c.Segment.MaxIndexBytes),
	); err != nil {
		return nil, lib.Wrap(err, "Unable to truncate file")
	}

	if idx.mmap, err = gommap.Map(
		idx.file.Fd(),
		gommap.PROT_READ|gommap.PROT_WRITE,
		gommap.MAP_SHARED,
	); err != nil {
		return nil, lib.Wrap(err, "Unable to create gommap map")
	}

	return idx, nil
}

// Takes an offset and returns associated record's position in the store.
func (i *Index) Read(in int64) (out uint32, pos uint64, err error) {
	if i.size == 0 {
		return 0, 0, lib.Wrap(io.EOF, "Unable read Index file(it's empty)")
	}

	if in == -1 {
		out = uint32((i.size / entWidth) - 1)
	} else {
		out = uint32(in)
	}

	pos = uint64(out) * entWidth

	if i.size < pos+entWidth {
		return 0, 0, lib.Wrap(io.EOF, "Position is out of file's amount")
	}

	out = enc.Uint32(i.mmap[pos : pos+offWidth])
	pos = enc.Uint64(i.mmap[pos+offWidth : pos+entWidth])

	return
}

// Appends the given offset and position to the Index file.
func (i *Index) Write(off uint32, pos uint64) error {
	if uint64(len(i.mmap)) < i.size+entWidth {
		return lib.Wrap(io.EOF, "Not enough space to append index data")
	}

	enc.PutUint32(i.mmap[i.size:i.size+offWidth], off)
	enc.PutUint64(i.mmap[i.size+offWidth:i.size+entWidth], pos)

	i.size += entWidth

	return nil
}

// Males sure the memory-mapped file has synced its data and flushed data to stable storage.
// Then it truncates file to the amount of data that's actually in it and close the file.
func (i *Index) Close() error {
	if err := i.mmap.Sync(gommap.MS_SYNC); err != nil {
		return lib.Wrap(err, "Unable to sync mmap")
	}

	if err := i.file.Sync(); err != nil {
		return lib.Wrap(err, "Unable to sync file")
	}

	if err := i.file.Truncate(int64(i.size)); err != nil {
		return lib.Wrap(err, "Unable to truncate file")
	}

	return i.file.Close()
}

// Returns Index's file path.
func (i *Index) Name() string {
	return i.file.Name()
}

// Copyright sasha.los.0148@gmail.com
// All Rights have been taken by Mafia :)
package log

import (
	"io"
	api "log/api/v1"
	"log/lib"
	"os"

	"io/ioutil"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Log controls over segments.
type Log struct {
	mu sync.RWMutex

	Dir    string
	Config Config

	activeSegment *Segment
	segments      []*Segment
}

// Creates new segment -> appends the segment -> makes it active
func (l *Log) newSegment(off uint64) error {
	s, err := NewSegment(l.Dir, off, l.Config)
	if err != nil {
		return lib.Wrap(err, "Unable to create a segment")
	}

	l.segments = append(l.segments, s)
	l.activeSegment = s
	return nil
}

// Set up itself for the segments that already exist on disk or bootstrapping the initial segment.
func NewLog(dir string, c Config) (*Log, error) {
	var baseOffsets []uint64

	// Default configs
	if c.Segment.MaxStoreBytes == 0 {
		c.Segment.MaxStoreBytes = 1024
	}

	if c.Segment.MaxIndexBytes == 0 {
		c.Segment.MaxIndexBytes = 1024
	}

	l := &Log{
		Dir:    dir,
		Config: c,
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, lib.Wrap(err, "Unable to read directory")
	}

	for _, file := range files {
		offStr := strings.TrimSuffix(
			file.Name(),
			path.Ext(file.Name()),
		)
		off, err := strconv.ParseUint(offStr, 10, 0)
		if err != nil {
			return nil, lib.Wrap(err, "Unable to get offset")
		}
		baseOffsets = append(baseOffsets, off)
	}

	sort.Slice(baseOffsets, func(i, j int) bool {
		return baseOffsets[i] < baseOffsets[j]
	})

	for i := 0; i < len(baseOffsets); i++ {
		if err := l.newSegment(baseOffsets[i]); err != nil {
			return nil, lib.Wrap(err, "Unable to create new segment")
		}
		// baseOffset contains dup for index and store so we skip
		// the dup
		i++
	}

	if l.segments == nil {
		if err := l.newSegment(c.Segment.InitialOffset); err != nil {
			return nil, lib.Wrap(err, "Unable to create segment with initial offset")
		}
	}

	return l, nil
}

// Appends record to the active segment and trying to create new segment if active is full.
func (l *Log) Append(record *api.Record) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	off, err := l.activeSegment.Append(record)
	if err != nil {
		return 0, lib.Wrap(err, "Unable to append record to the active segment")
	}

	if l.activeSegment.IsMaxed() {
		err = l.newSegment(off + 1)
	}

	return off, err
}

// Reads the record stored at the given offset.
func (l *Log) Read(off uint64) (*api.Record, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var s *Segment

	for _, segment := range l.segments {
		if segment.baseOffset <= off && off < segment.nextOffset {
			s = segment
			break
		}
	}

	if s == nil || s.nextOffset <= off {
		return nil, api.ErrOffsetOutOfRange{Offset: off}
	}

	return s.Read(off)
}

// Closes all segments at Log.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, segment := range l.segments {
		if err := segment.Close(); err != nil {
			return lib.Wrap(err, "Unable to close Log due the error of Segment closing")
		}
	}

	return nil
}

// Closes Log and Remove the whole directory.
func (l *Log) Remove() error {
	if err := l.Close(); err != nil {
		return lib.Wrap(err, "Unable to remove Log due the error of Log closing")
	}

	return os.RemoveAll(l.Dir)
}

// Removes and creates new log to replace it.
func (l *Log) Reset() (err error) {
	if err := l.Remove(); err != nil {
		return lib.Wrap(err, "Unable to reset Log due the error of removing")
	}

	log, err := NewLog(l.Dir, l.Config)
	if err != nil {
		return lib.Wrap(err, "Unable to create new Log due to restore")
	}

	*l = *log
	return
}

// Returns the first segment's base offset.
func (l *Log) LowestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.segments[0].baseOffset, nil
}

// Returns the latest segment's offset.
func (l *Log) HighestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	off := l.segments[len(l.segments)-1].nextOffset
	if off == 0 {
		return 0, nil
	}

	return off - 1, nil
}

// Removes all segments whose highest offset is lower than lowest.
func (l *Log) Truncate(lowest uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var segments []*Segment

	for _, s := range l.segments {
		if s.nextOffset <= lowest+1 {
			if err := s.Remove(); err != nil {
				return lib.Wrap(err, "Unable to remove Segment due the truncation")
			}
			continue
		}
		segments = append(segments, s)
	}

	l.segments = segments
	return nil
}

type originReader struct {
	*Store
	off int64
}

// originReader should implement Reader interface.
// Simple call of ReadAt from Store.
func (o *originReader) Read(p []byte) (int, error) {
	n, err := o.ReadAt(p, o.off)
	o.off += int64(n)
	return n, err
}

// Returns io.Reader to read the whole log.
func (l *Log) Reader() io.Reader {
	l.mu.RLock()
	defer l.mu.RUnlock()

	readers := make([]io.Reader, len(l.segments))

	for i, segment := range l.segments {
		readers[i] = &originReader{segment.store, 0}
	}

	return io.MultiReader(readers...)
}

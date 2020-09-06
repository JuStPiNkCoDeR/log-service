// Copyright sasha.los.0148@gmail.com
// All Rights have been taken by Mafia :)
package log

import (
	"github.com/stretchr/testify/require"
	api "mafia/log/api/v1"

	"io/ioutil"
	"os"
	"testing"
)

func TestSegment(t *testing.T) {
	dir, err := ioutil.TempDir("", "segment-test")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			panic(err)
		}
	}()

	want := &api.Record{Value: []byte("hello world")}

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = entWidth * 3

	s, err := NewSegment(dir, 16, c)
	require.NoError(t, err)
	require.Equal(t, uint64(16), s.nextOffset, s.nextOffset)
	require.False(t, s.IsMaxed())

	for i := uint64(0); i < 3; i++ {
		off, err := s.Append(want)
		require.NoError(t, err)
		require.Equal(t, 16+i, off)

		got, err := s.Read(off)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}

	_, err = s.Append(want)
	require.Error(t, err)
	// maxed index
	require.True(t, s.IsMaxed())

	c.Segment.MaxStoreBytes = uint64(len(want.Value) * 3)
	c.Segment.MaxIndexBytes = 1024

	s, err = NewSegment(dir, 16, c)
	require.NoError(t, err)
	// maxed store
	require.True(t, s.IsMaxed())

	err = s.Remove()
	require.NoError(t, err)

	s, err = NewSegment(dir, 16, c)
	require.NoError(t, err)
	require.False(t, s.IsMaxed())
}

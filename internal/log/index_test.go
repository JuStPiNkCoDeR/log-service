// Copyright sasha.los.0148@gmail.com
// All Rights have been taken by Mafia :)
package log

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndex(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "index_test")
	require.NoError(t, err)
	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			panic(err)
		}
	}()

	c := Config{}
	c.Segment.MaxIndexBytes = 1024
	idx, err := NewIndex(f, c)
	require.NoError(t, err)

	_, _, err = idx.Read(-1)
	require.Error(t, err)
	require.Equal(t, f.Name(), idx.Name())

	entries := []struct {
		Off uint32
		Pos uint64
	}{
		{Off: 0, Pos: 0},
		{Off: 1, Pos: 10},
	}

	for _, want := range entries {
		err = idx.Write(want.Off, want.Pos)
		require.NoError(t, err)

		_, pos, err := idx.Read(int64(want.Off))
		require.NoError(t, err)
		require.Equal(t, want.Pos, pos)
	}
	// index and scanner should error when reading past existing entries
	_, _, err = idx.Read(int64(len(entries)))
	require.Error(t, err)
	_ = idx.Close()
	// index should build its state from existing file
	f, _ = os.OpenFile(f.Name(), os.O_RDWR, 0600)
	idx, err = NewIndex(f, c)
	require.NoError(t, err)

	off, pos, err := idx.Read(-1)
	require.NoError(t, err)
	require.Equal(t, uint32(1), off)
	require.Equal(t, entries[1].Pos, pos)
}

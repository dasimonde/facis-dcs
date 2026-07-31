package codec_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status/codec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadStatusValue_MSBExamples(t *testing.T) {
	cases := []struct {
		data  []byte
		index uint64
		width uint
		want  uint64
	}{
		{[]byte{0x80}, 0, 1, 1},
		{[]byte{0x40}, 1, 1, 1},
		{[]byte{0x01}, 7, 1, 1},
		{[]byte{0x00, 0x80}, 8, 1, 1},
		{[]byte{0x00}, 0, 1, 0},
		{[]byte{0xC0}, 0, 2, 3},
	}
	for _, tc := range cases {
		got, err := codec.ReadStatusValue(tc.data, tc.index, tc.width, codec.MSBFirst)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "index %d width %d", tc.index, tc.width)
	}
}

func TestReadStatusValue_LSBExamples(t *testing.T) {
	cases := []struct {
		data  []byte
		index uint64
		width uint
		want  uint64
	}{
		{[]byte{0x01}, 0, 1, 1},
		{[]byte{0x02}, 1, 1, 1},
		{[]byte{0x80}, 7, 1, 1},
		{[]byte{0x00, 0x01}, 8, 1, 1},
		{[]byte{0x00}, 0, 1, 0},
		{[]byte{0x03}, 0, 2, 3},
	}
	for _, tc := range cases {
		got, err := codec.ReadStatusValue(tc.data, tc.index, tc.width, codec.LSBFirst)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "index %d width %d", tc.index, tc.width)
	}
}

func TestReadStatusValue_RejectsOutOfRange(t *testing.T) {
	_, err := codec.ReadStatusValue([]byte{0x01}, 8, 1, codec.LSBFirst)
	require.Error(t, err)
}

func TestReadStatusValue_RejectsInvalidWidth(t *testing.T) {
	_, err := codec.ReadStatusValue([]byte{0x01}, 0, 0, codec.LSBFirst)
	require.Error(t, err)

	_, err = codec.ReadStatusValue([]byte{0x01}, 0, 9, codec.LSBFirst)
	require.Error(t, err)
}

func TestReadStatusValue_MSBvsLSB_DifferentAtSameIndex(t *testing.T) {
	data := []byte{0x20}

	msb, err := codec.ReadStatusValue(data, 2, 1, codec.MSBFirst)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), msb)

	lsb, err := codec.ReadStatusValue(data, 5, 1, codec.LSBFirst)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), lsb)

	msbClear, err := codec.ReadStatusValue(data, 5, 1, codec.MSBFirst)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), msbClear)

	lsbClear, err := codec.ReadStatusValue(data, 2, 1, codec.LSBFirst)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), lsbClear)
}

func TestEntryCount(t *testing.T) {
	assert.Equal(t, uint64(64), codec.EntryCount(make([]byte, 8), 1))
	assert.Equal(t, uint64(32), codec.EntryCount(make([]byte, 8), 2))
}

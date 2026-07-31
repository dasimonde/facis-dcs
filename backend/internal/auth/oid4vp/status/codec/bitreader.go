package codec

import (
	"errors"
)

var (
	ErrInvalidStatusSize   = errors.New("invalid status size")
	ErrIndexOutOfRange     = errors.New("status index out of range")
	ErrUnsupportedBitOrder = errors.New("unsupported bit order")
)

type BitOrder uint8

const (
	MSBFirst BitOrder = iota
	LSBFirst
)

func ReadStatusValue(
	data []byte,
	index uint64,
	width uint,
	order BitOrder,
) (uint64, error) {
	if width == 0 || width > 8 {
		return 0, ErrInvalidStatusSize
	}

	startBit := index * uint64(width)
	endBit := startBit + uint64(width)

	if endBit > uint64(len(data))*8 {
		return 0, ErrIndexOutOfRange
	}

	var result uint64

	for offset := uint(0); offset < width; offset++ {
		absoluteBit := startBit + uint64(offset)
		byteIndex := absoluteBit / 8
		bitInByte := uint(absoluteBit % 8)

		var bit uint8

		switch order {
		case MSBFirst:
			bit = (data[byteIndex] >> (7 - bitInByte)) & 1
			result = (result << 1) | uint64(bit)
		case LSBFirst:
			bit = (data[byteIndex] >> bitInByte) & 1
			result |= uint64(bit) << offset
		default:
			return 0, ErrUnsupportedBitOrder
		}
	}

	return result, nil
}

func EntryCount(data []byte, width uint) uint64 {
	if width == 0 {
		return 0
	}
	return uint64(len(data)) * 8 / uint64(width)
}

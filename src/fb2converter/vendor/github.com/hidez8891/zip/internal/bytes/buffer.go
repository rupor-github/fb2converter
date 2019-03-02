package bytes

import (
	goBytes "bytes"
)

type BufferAt struct {
	goBytes.Buffer
}

func (b *BufferAt) WriteAt(p []byte, off int64) (int, error) {
	size := int64(len(p)) + off
	if int64(b.Len()) < size {
		padsize := size - int64(b.Len())
		padding := make([]byte, padsize)

		if _, err := b.Write(padding); err != nil {
			return 0, err
		}
	}

	n := copy(b.Bytes()[off:], p)
	return n, nil
}

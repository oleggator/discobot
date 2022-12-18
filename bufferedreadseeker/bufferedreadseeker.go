package bufferedreadseeker

import (
	"errors"
	"io"
)

const (
	defaultBufSize = 4096
)

// Reader implements buffering for an io.Reader object.
type Reader struct {
	buf    []byte
	rd     io.Reader // reader provided by the client
	offset int       // buf read and write positions
	err    error
}

var _ io.ReadSeeker = &Reader{}

func NewReader(rd io.Reader) *Reader {
	return NewReaderWithSize(rd, defaultBufSize)
}

func NewReaderWithSize(rd io.Reader, n int) *Reader {
	return &Reader{
		buf:    make([]byte, 0, n),
		rd:     rd,
		offset: 0,
		err:    nil,
	}
}

var errNegativeRead = errors.New("bufio: reader returned negative count from Read")

func (b *Reader) readErr() error {
	err := b.err
	b.err = nil
	return err
}

// Read reads data into p.
// It returns the number of bytes read into p.
// The bytes are taken from at most one Read on the underlying Reader,
// hence n may be less than len(p).
// To read exactly len(p) bytes, use io.ReadFull(b, p).
// If the underlying Reader can return a non-zero count with io.EOF,
// then this Read method can do so as well; see the [io.Reader] docs.
func (b *Reader) Read(out []byte) (n int, err error) {
	bufLen := len(b.buf)
	remainingData := bufLen - b.offset

	if len(out) <= remainingData {
		n = copy(out, b.buf[b.offset:b.offset+len(out)])
		b.offset += n
		return n, nil
	}

	if _, err := b.fetchNewData(len(out) - remainingData); err != nil {
		return 0, err
	}

	n = copy(out, b.buf[b.offset:])
	b.offset += n

	return n, nil
}

func (b *Reader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int
	switch whence {
	case io.SeekStart:
		newOffset = int(offset)
	case io.SeekCurrent:
		newOffset = b.offset + int(offset)
	default:
		return 0, errors.New("SeekStart and SeekCurrent are only supported whences")
	}

	if newOffset > len(b.buf) {
		if _, err := b.fetchNewData(newOffset - len(b.buf)); err != nil {
			return 0, err
		}
	}

	b.offset = newOffset

	return int64(b.offset), nil
}

func (b *Reader) grow(n int) int {
	l := len(b.buf)
	if n <= cap(b.buf)-l {
		b.buf = b.buf[:l+n]
		return l
	}

	newBuf := make([]byte, l+n)
	copy(newBuf, b.buf)
	b.buf = newBuf
	return l
}

func (b *Reader) fetchNewData(size int) (int, error) {
	oldBufLen := len(b.buf)
	b.grow(size)

	var n int
	n, b.err = b.rd.Read(b.buf[oldBufLen:])
	if n < 0 {
		panic(errNegativeRead)
	}
	if n == 0 {
		return 0, b.readErr()
	}
	b.buf = b.buf[:oldBufLen+n]

	return n, nil
}

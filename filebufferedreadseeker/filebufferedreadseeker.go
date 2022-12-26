package filebufferedreadseeker

import (
	"errors"
	"io"
	"os"
)

// Reader implements buffering for an io.Reader object.
type Reader struct {
	file *os.File
	rd   io.Reader // reader provided by the client
	err  error
}

var _ io.ReadSeeker = &Reader{}
var errNegativeRead = errors.New("bufio: reader returned negative count from Read")

func NewReader(rd io.Reader) (*Reader, error) {
	file, err := os.CreateTemp("", "discobot_*.tmp")
	if err != nil {
		return nil, err
	}

	return &Reader{
		file: file,
		rd:   rd,
		err:  nil,
	}, nil
}

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
	offset, err := b.Offset()
	if err != nil {
		return 0, err
	}

	fileLen, err := b.Len()
	if err != nil {
		return 0, err
	}

	if remainingData := fileLen - offset; int64(len(out)) <= remainingData {
		n, err := b.file.Read(out)
		if err != nil {
			return 0, err
		}

		return n, nil
	}

	n, b.err = b.rd.Read(out)
	if n < 0 {
		panic(errNegativeRead)
	}
	if n == 0 {
		return 0, b.readErr()
	}

	fileN, err := b.file.Write(out[:n])
	if err != nil {
		return 0, err
	}
	if fileN != n {
		panic("insufficient write")
	}

	return n, nil
}

func (b *Reader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		oldOffset, err := b.Offset()
		if err != nil {
			return 0, err
		}
		newOffset = oldOffset + offset
	default:
		return 0, errors.New("SeekStart and SeekCurrent are only supported whences")
	}

	fileLen, err := b.Len()
	if err != nil {
		return 0, err
	}

	if newOffset <= fileLen {
		offset, err := b.file.Seek(newOffset, io.SeekStart)
		if err != nil {
			return 0, err
		}
		return offset, err
	}

	dataToFetch := newOffset - fileLen
	buf := make([]byte, dataToFetch)

	var n int
	n, b.err = io.ReadFull(b.rd, buf)
	if n < 0 {
		panic(errNegativeRead)
	}
	if n == 0 {
		return 0, b.readErr()
	}

	fileN, err := b.file.Write(buf[:n])
	if err != nil {
		return 0, err
	}
	if fileN != n {
		panic("insufficient write")
	}

	newOffset, err = b.Offset()
	if err != nil {
		return 0, err
	}

	return newOffset, nil
}

func (b *Reader) Offset() (int64, error) {
	return b.file.Seek(0, io.SeekCurrent)
}

func (b *Reader) Len() (int64, error) {
	fileStat, err := b.file.Stat()
	if err != nil {
		return 0, err
	}

	fileLen := fileStat.Size()
	return fileLen, err
}

func (b *Reader) Close() error {
	fileInfo, err := b.file.Stat()
	if err != nil {
		return err
	}
	if err := b.file.Close(); err != nil {
		return err
	}

	if err := os.Remove(fileInfo.Name()); err != nil {
		return err
	}

	return nil
}

package ogg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

var endian = binary.BigEndian

type HeaderType uint8

func (ht HeaderType) String() string {
	flags := make([]string, 0, 3)
	if ht&ContinuationFlag != 0 {
		flags = append(flags, "ContinuationFlag")
	}
	if ht&BeginningOfStreamFlag != 0 {
		flags = append(flags, "BeginningOfStreamFlag")
	}
	if ht&EndOfStreamFlag != 0 {
		flags = append(flags, "EndOfStreamFlag")
	}

	return strings.Join(flags, "|")
}

const (
	ContinuationFlag HeaderType = 1 << iota
	BeginningOfStreamFlag
	EndOfStreamFlag
)

type PageDecoder struct {
	r io.Reader
}

func NewPageDecoder(r io.Reader) PageDecoder {
	return PageDecoder{r: r}
}

func (d *PageDecoder) NextPage() (*Page, error) {
	capturePattern := make([]byte, 4)
	if _, err := io.ReadFull(d.r, capturePattern); err != nil {
		return nil, err
	}
	if !bytes.Equal(capturePattern, []byte("OggS")) {
		return nil, fmt.Errorf("invalid format: %s", string(capturePattern))
	}

	var version uint8
	if err := binary.Read(d.r, endian, &version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if version != 0 {
		return nil, errors.New("invalid version")
	}

	pd := &Page{}
	if err := binary.Read(d.r, endian, &pd.HeaderType); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	if err := binary.Read(d.r, endian, &pd.Grantule); err != nil {
		return nil, fmt.Errorf("failed to read grantule: %w", err)
	}
	if err := binary.Read(d.r, endian, &pd.Serial); err != nil {
		return nil, fmt.Errorf("failed to read serial: %w", err)
	}
	if err := binary.Read(d.r, endian, &pd.Sequence); err != nil {
		return nil, fmt.Errorf("failed to read sequence: %w", err)
	}
	if err := binary.Read(d.r, endian, &pd.Checksum); err != nil {
		return nil, fmt.Errorf("failed to read checksum: %w", err)
	}
	if err := binary.Read(d.r, endian, &pd.NumberOfSegments); err != nil {
		return nil, fmt.Errorf("failed to read number of segments: %w", err)
	}

	pd.PageSize = 0
	pd.SegmentSizes = make([]uint8, pd.NumberOfSegments)
	for i := range pd.SegmentSizes {
		if err := binary.Read(d.r, endian, &pd.SegmentSizes[i]); err != nil {
			return nil, fmt.Errorf("failed to read segment size %d/%d: %w", i, pd.NumberOfSegments, err)
		}
		pd.PageSize += uint32(pd.SegmentSizes[i])
	}

	pd.r = &io.LimitedReader{R: d.r, N: int64(pd.PageSize)}

	return pd, nil
}

type Page struct {
	r             *io.LimitedReader
	segmentCursor int

	HeaderType       HeaderType
	Grantule         uint64
	Serial           uint32
	Sequence         uint32
	Checksum         uint32
	NumberOfSegments uint8
	SegmentSizes     []uint8
	PageSize         uint32
}

func (pd *Page) NextSegement() ([]byte, error) {
	if pd.segmentCursor >= int(pd.NumberOfSegments) {
		return nil, io.EOF
	}

	segment := make([]byte, pd.SegmentSizes[pd.segmentCursor])
	if _, err := io.ReadFull(pd.r, segment); err != nil {
		return nil, err
	}

	pd.segmentCursor++

	return segment, nil
}

type PacketDecoder struct {
	pd      *PageDecoder
	page    *Page
	buffers map[uint32]*bytes.Buffer
}

func NewPacketDecoder(r io.Reader) *PacketDecoder {
	pd := NewPageDecoder(r)

	return &PacketDecoder{
		pd:      &pd,
		buffers: make(map[uint32]*bytes.Buffer),
	}
}

func (d *PacketDecoder) NextPacket() (io.ReadCloser, error) {
	if d.page == nil {
		if err := d.nextPage(); err != nil {
			return nil, err
		}
	}

	for {
		segment, err := d.page.NextSegement()
		if errors.Is(err, io.EOF) {
			if err := d.nextPage(); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}

		buffer := d.getStreamBuffer(d.page.Serial)
		buffer.Write(segment)
		if len(segment) == 255 {
			continue
		}

		return &packet{buf: buffer, stream: d.page.Serial}, nil
	}
}

func (d *PacketDecoder) getStreamBuffer(serial uint32) *bytes.Buffer {
	buffer, ok := d.buffers[serial]
	if !ok {
		buffer = &bytes.Buffer{}
		d.buffers[serial] = buffer
	}

	return buffer
}

func (d *PacketDecoder) nextPage() error {
	var err error
	d.page, err = d.pd.NextPage()

	if errors.Is(err, io.EOF) {
		for _, buffer := range d.buffers {
			if buffer.Len() != 0 {
				return io.ErrUnexpectedEOF
			}
		}
	}

	return err

}

type packet struct {
	buf    *bytes.Buffer
	stream uint32
}

func (p *packet) Read(b []byte) (int, error) {
	return p.buf.Read(b)
}

func (p *packet) Close() error {
	p.buf.Reset()
	return nil
}

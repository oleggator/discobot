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

type Page struct {
	Version    uint8
	HeaderType HeaderType
	Grantule   uint64
	Serial     uint32
	Sequence   uint32
	Checksum   uint32

	NumberOfSegments uint8
	SegmentSizes     []uint8
	Segments         [][]byte
}

type HeaderType uint8

const (
	ContinuationFlag HeaderType = 1 << iota
	BeginningOfStreamFlag
	EndOfStreamFlag
)

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

	page := &Page{}
	if err := binary.Read(d.r, endian, &page.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if page.Version != 0 {
		return nil, errors.New("invalid version")
	}

	if err := binary.Read(d.r, endian, &page.HeaderType); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	if err := binary.Read(d.r, endian, &page.Grantule); err != nil {
		return nil, fmt.Errorf("failed to read grantule: %w", err)
	}
	if err := binary.Read(d.r, endian, &page.Serial); err != nil {
		return nil, fmt.Errorf("failed to read serial: %w", err)
	}
	if err := binary.Read(d.r, endian, &page.Sequence); err != nil {
		return nil, fmt.Errorf("failed to read sequence: %w", err)
	}
	if err := binary.Read(d.r, endian, &page.Checksum); err != nil {
		return nil, fmt.Errorf("failed to read checksum: %w", err)
	}
	if err := binary.Read(d.r, endian, &page.NumberOfSegments); err != nil {
		return nil, fmt.Errorf("failed to read number of segments: %w", err)
	}

	page.SegmentSizes = make([]uint8, page.NumberOfSegments)
	if _, err := io.ReadFull(d.r, page.SegmentSizes); err != nil {
		return nil, err
	}

	segmentsSize := 0
	for _, segmentSize := range page.SegmentSizes {
		segmentsSize += int(segmentSize)
	}

	segmentsBin := make([]byte, segmentsSize)
	if _, err := io.ReadFull(d.r, segmentsBin); err != nil {
		return nil, err
	}

	acc := 0
	page.Segments = make([][]byte, page.NumberOfSegments)
	for i, segmentSize := range page.SegmentSizes {
		segmentSize := int(segmentSize)
		page.Segments[i] = segmentsBin[acc : acc+segmentSize]
		acc += segmentSize
	}

	return page, nil
}

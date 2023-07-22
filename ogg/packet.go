package ogg

import (
	"io"
	"net"
)

type Packet struct {
	buffers *net.Buffers
	Stream  uint32
}

func (p *Packet) Read(b []byte) (n int, err error) {
	return p.buffers.Read(b)
}

func (p *Packet) WriteTo(w io.Writer) (n int64, err error) {
	return p.buffers.WriteTo(w)
}

type PacketDecoder struct {
	pd              *PageDecoder
	segmentCursor   int
	page            *Page
	buffersByStream map[uint32]*net.Buffers
}

func NewPacketDecoder(r io.Reader) *PacketDecoder {
	pd := NewPageDecoder(r)

	return &PacketDecoder{
		pd:              &pd,
		buffersByStream: make(map[uint32]*net.Buffers),
	}
}

func (d *PacketDecoder) NextPacket() (*Packet, error) {
	for {
		segment, err := d.nextSegment()
		if err != nil {
			return nil, err
		}

		buffers := d.getStreamBuffer(d.page.Serial)
		*buffers = append(*buffers, segment)

		if len(segment) < 255 {
			return &Packet{buffers: buffers, Stream: d.page.Serial}, nil
		}
	}
}

func (d *PacketDecoder) getStreamBuffer(serial uint32) *net.Buffers {
	buffersRef, ok := d.buffersByStream[serial]
	if !ok {
		buffers := make(net.Buffers, 0, 2)
		buffersRef = &buffers
		d.buffersByStream[serial] = buffersRef
	}

	return buffersRef
}

func (d *PacketDecoder) nextPage() error {
	var err error
	d.page, err = d.pd.NextPage()

	//if errors.Is(err, io.EOF) {
	//	for _, buffer := range d.buffersByStream {
	//		if buffer.Len() != 0 {
	//			return io.ErrUnexpectedEOF
	//		}
	//	}
	//}

	d.segmentCursor = 0

	return err
}

func (d *PacketDecoder) nextSegment() ([]byte, error) {
	if d.page == nil || d.segmentCursor >= int(d.page.NumberOfSegments) {
		if err := d.nextPage(); err != nil {
			return nil, err
		}
	}

	segment := d.page.Segments[d.segmentCursor]
	d.segmentCursor++

	return segment, nil
}

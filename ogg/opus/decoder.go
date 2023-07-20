// https://datatracker.ietf.org/doc/html/rfc7845.html
package opus

import (
	"bytes"
	"discobot/ogg"
	"encoding/binary"
	"fmt"
	"io"
)

// little endian (really)
var endian = binary.LittleEndian

// https://datatracker.ietf.org/doc/html/rfc7845.html#section-3
// https://datatracker.ietf.org/doc/html/rfc7845.html#section-5
type OpusDecorder struct {
	pd *ogg.PacketDecoder

	outputChannels       uint8
	preSkip              uint16
	inputSampleRate      uint32
	outputGain           int16
	channelMappingFamily uint8

	vendor       string
	userComments []string
}

func NewOpusDecoder(r io.Reader) (*OpusDecorder, error) {
	od := &OpusDecorder{pd: ogg.NewPacketDecoder(r)}
	if err := od.readIdentificationHeader(); err != nil {
		return nil, err
	}
	if err := od.readTags(); err != nil {
		return nil, err
	}

	return od, nil
}

func (od *OpusDecorder) NextPacket() (io.ReadCloser, error) {
	return od.pd.NextPacket()
}

func (od *OpusDecorder) readIdentificationHeader() error {
	rc, err := od.pd.NextPacket()
	if err != nil {
		return err
	}
	defer rc.Close()

	capturePattern := make([]byte, 8)
	if _, err := io.ReadFull(rc, capturePattern); err != nil {
		return err
	}
	if !bytes.Equal(capturePattern, []byte("OpusHead")) {
		return fmt.Errorf("invalid format: %s", string(capturePattern))
	}

	var version uint8
	if err := binary.Read(rc, endian, &version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("invalid version: %d", version)
	}

	if err := binary.Read(rc, endian, &od.outputChannels); err != nil {
		return err
	}
	if od.outputChannels == 0 {
		return fmt.Errorf("invalid channels number: %d", od.outputChannels)
	}

	if err := binary.Read(rc, endian, &od.preSkip); err != nil {
		return err
	}
	if err := binary.Read(rc, endian, &od.inputSampleRate); err != nil {
		return err
	}
	if err := binary.Read(rc, endian, &od.outputGain); err != nil {
		return err
	}
	if err := binary.Read(rc, endian, &od.channelMappingFamily); err != nil {
		return err
	}

	return nil
}

func (od *OpusDecorder) readTags() error {
	rc, err := od.pd.NextPacket()
	if err != nil {
		return err
	}
	defer rc.Close()

	capturePattern := make([]byte, 8)
	if _, err := io.ReadFull(rc, capturePattern); err != nil {
		return err
	}
	if !bytes.Equal(capturePattern, []byte("OpusTags")) {
		return fmt.Errorf("invalid format: %s", string(capturePattern))
	}

	var vendorStringLength uint32
	if err := binary.Read(rc, endian, &vendorStringLength); err != nil {
		return err
	}
	vendor := make([]byte, vendorStringLength)
	if _, err := io.ReadFull(rc, vendor); err != nil {
		return err
	}
	od.vendor = string(vendor)

	var userCommentListLen uint32
	if err := binary.Read(rc, endian, &userCommentListLen); err != nil {
		return err
	}
	od.userComments = make([]string, userCommentListLen)
	for i := range od.userComments {
		var userCommentLen uint32
		if err := binary.Read(rc, endian, &userCommentLen); err != nil {
			return err
		}
		userComment := make([]byte, userCommentLen)
		if _, err := io.ReadFull(rc, userComment); err != nil {
			return err
		}
		od.userComments[i] = string(userComment)
	}

	return nil
}

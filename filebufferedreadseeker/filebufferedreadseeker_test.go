package filebufferedreadseeker_test

import (
	"bytes"
	"discobot/filebufferedreadseeker"
	"io"
	"log"
	"testing"
)

func TestFileBufferedReadSeeker(t *testing.T) {
	someBuf := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	r := bytes.NewReader(someBuf)

	bufR, err := filebufferedreadseeker.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}

	for {
		buf := make([]byte, 7)
		n, err := bufR.Read(buf)
		if err != nil {
			log.Println(err)
			return
		}
		log.Println(n, buf)
	}
}

func TestFileBufferedReadSeekerSeek(t *testing.T) {
	someBuf := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	r := bytes.NewReader(someBuf)

	bufR, err := filebufferedreadseeker.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 7)

	n, err := bufR.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(n, buf)

	n, err = bufR.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(n, buf)

	bufR.Seek(0, io.SeekStart)

	n, err = bufR.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(n, buf)

}

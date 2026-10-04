package unzip

import (
	"archive/zip"
	"hash"
	"io"
)

type CRCReader struct {
	io.Reader
	Hash hash.Hash32
	Crc  *uint32
}

// Read reads byte stream into the hash and validates the sum of the hash
func (r *CRCReader) Read(b []byte) (n int, err error) {
	n, err = r.Reader.Read(b)
	r.Hash.Write(b[:n])
	if err == nil {
		return
	}
	if err == io.EOF {
		if r.Crc != nil && *r.Crc != 0 && r.Hash.Sum32() != *r.Crc {
			err = zip.ErrChecksum
		}
	}
	return
}


package unzip

import (
	"compress/flate"
	"io"
	"io/ioutil"
	"sync"
)

// Decompressor wraps a reader with a decompressing Reader.
// The decompressed ReadCloser is returned to callers who open files from within
// the archive.
type Decompressor func(io.Reader) io.ReadCloser

var (
	rmu sync.RWMutex // guard mutex for compressor and decompressor maps

	decompressors = map[uint16]Decompressor{
		Store:   ioutil.NopCloser,
		Deflate: flate.NewReader,
	}
)

// RegisterDecompressor allows custom decompressors for a specified method ID.
func RegisterDecompressor(method uint16, d Decompressor) {
	rmu.Lock()
	defer rmu.Unlock()

	if _, ok := decompressors[method]; ok {
		panic("decompressor already registered")
	}
	decompressors[method] = d
}

func decompressor(method uint16) Decompressor {
	rmu.RLock()
	defer rmu.RUnlock()
	return decompressors[method]
}

func GetDecompressor(method uint16) Decompressor {
	return decompressor(method)
}

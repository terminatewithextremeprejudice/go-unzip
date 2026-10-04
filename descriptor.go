package unzip

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
)

// Read data desciptor from byte array and set the values to zip FileHeader accordingly

type DescriptorReader struct {
	br         *bufio.Reader
	size       uint64
	eof        bool
	fileHeader *zip.FileHeader
}

func (r *DescriptorReader) Read(p []byte) (n int, err error) {
	if r.eof {
		return 0, io.EOF
	}

	if n = len(p); n > maxRead {
		n = maxRead
	}

	// allocate new buffer of static size
	z, err := r.br.Peek(n + readAhead)
	if err != nil {
		if err == io.EOF && len(z) < 46+22 { // Min length of the central directory + end of central directory
			return 0, err
		}
		n = len(z)
	}

	// Look for header of next file or central directory
	discard := n
	s := dataDescriptorLen

	// Search for data descriptor from the buffer
	for !r.eof && s < n {
		if s > len(z)-4 {
			break
		}

		// Check if byte array contains any of the interesting signature bytes
		i := bytes.Index(z[s:], sigBytes) // + s
		if i == -1 {
			k := s

			if k < dataDescriptorLen {
				k = dataDescriptorLen
			}

			j := bytes.Index(z[k-dataDescriptorLen:s], sigBytes)
			if j > -1 {
				n = n - j
				discard = n
				i += j
			}

			break
		}

		i += s

		// too little data was read from buffer, or no signature was found from buffer
		// discard data except for the last byte (as it might form other half of the signature)
		if i+4 > len(z) {
			discard = n
			break
		}

		// Check if directoryHeaderSignature or fileHeaderSignature (0x04034b50) exists in buffer
		if sig := binary.LittleEndian.Uint32(z[i : i+4]); sig == fileHeaderDeflateSignature || sig == directoryHeaderSignature {
			// Check for compressed file sizes
			if i < len(z)-8 { // Zip32
				offset := 0

				// check if the optional data descriptor signature is actually set at beginning of descriptor
				// if so, skip it
				if i >= dataDescriptorLen && binary.LittleEndian.Uint32(z[i-dataDescriptorLen:i-(dataDescriptorLen-4)]) == dataDescriptorSignature {
					offset = 4
				}

				// Zip32 compressed file size,
				// trust the fileheader signature to be an actual header
				// if written bytecount matches with the actual compressed file size header at offset[0:4] OR
				// if the header info has been incorrect but header has been followed by datadescriptor signature
				if binary.LittleEndian.Uint32(z[i-8:i-4]) == uint32(r.size)+uint32(i-12-offset) {
					n, discard = i-12-offset, i
					r.eof = true
					r.fileHeader.CRC32 = uint32LE(z[i-12 : i-8])
					r.fileHeader.CompressedSize = uint32LE(z[i-8 : i-4])
					r.fileHeader.UncompressedSize = uint32LE(z[i-4 : i])

					break
				}
}

			if i > dataDescriptor64Len { // Zip64
				// Optional dataDesciptorSignature
				offset := 0
				if binary.LittleEndian.Uint32(z[i-dataDescriptor64Len:i-(dataDescriptor64Len-4)]) == dataDescriptorSignature {
					offset = 4
				}

				// Zip64 compressed file size
				if i >= 8 && binary.LittleEndian.Uint64(z[i-16:i-8]) == r.size+uint64(i-20-offset) {
					n, discard = i-20-offset, i
					r.eof = true
					// We dont write CRC32 as there's no point on ZIP64
					r.fileHeader.CompressedSize64 = uint64LE(z[i-16 : i-8])
					r.fileHeader.UncompressedSize64 = uint64LE(z[i-8 : i])
					break
				}
			}
		}

		s = i + 2
		i += s
	}

	// Copy read bytes into description readers bufio.Reader
	copy(p, z[:n])
	r.size += uint64(n)
	r.br.Discard(discard)
	return
}

package unzip

import (
	"archive/zip"
	"bufio"
	"errors"
	"io"
)

const (
	readAhead  = 128
	maxRead    = 4096
	bufferSize = maxRead + readAhead
)

// Reader provides sequential access to the contents of a zip archive.
type Reader struct {
	io.Reader
	br         *bufio.Reader
	EOCDParsed bool
}

type readBuf []byte

type FileHeader struct {
	zip.FileHeader
	headerOffset int64
}

func (f *FileHeader) DataOffset(r io.ReaderAt) (offset int64, err error) {
	bodyOffset, err := f.findBodyOffset(r)
	if err != nil {
		return
	}
	return f.headerOffset + bodyOffset, nil
}

// findBodyOffset does the minimum work to verify the file has a header
// and returns the file body offset.
func (f *FileHeader) findBodyOffset(r io.ReaderAt) (int64, error) {
	var buf [fileHeaderLen]byte
	if _, err := r.ReadAt(buf[:], f.headerOffset); err != nil {
		return 0, err
	}
	b := readBuf(buf[:])
	if sig := b.uint32(); sig != fileHeaderSignature {
		return 0, errors.New("zip: invalid format")
	}
	b = b[22:] // skip over most of the header
	filenameLen := int(b.uint16())
	extraLen := int(b.uint16())
	return int64(fileHeaderLen + filenameLen + extraLen), nil
}

func (b *readBuf) uint16() uint16 {
	v := uint16LE(*b)
	*b = (*b)[2:]
	return v
}

func (b *readBuf) uint32() uint32 {
	v := uint32LE(*b)
	*b = (*b)[4:]
	return v
}

func (b *readBuf) uint64() uint64 {
	v := uint64LE(*b)
	*b = (*b)[8:]
	return v
}

func (b *readBuf) sub(n int) readBuf {
	b2 := (*b)[:n]
	*b = (*b)[n:]
	return b2
}

// Skip over the central directories data
func discardCentralDirectory(br *bufio.Reader) error {
	for {
		sigBytes, err := br.Peek(4)
		if err != nil {
			return err
		}

		switch sig := uint32LE(sigBytes); sig {
		case directoryHeaderSignature:
			if err := discardDirectoryHeaderRecord(br); err != nil {
				return err
			}
		case directoryEndSignature:
			if err := discardDirectoryEndRecord(br); err != nil {
				return err
			}
		case directory64EndSignature:
			if err := discardDirectory64EndRecord(br); err != nil {
				return err
			}
		case directory64LocSignature:
			if err := discardDirectory64LocSignature(br); err != nil {
				return err
			}
		default:
			return errors.New("zip: invalid format")
		}
	}
}

// Discard central directory header record
func discardDirectoryHeaderRecord(br *bufio.Reader) error {
	// Discard first 28 bytes of central directory ie. directory header
	if _, err := br.Discard(28); err != nil {
		return err
	}
	// Read next 6 bytes ie. signatures of the filename, extra and comment lengths
	lb, err := br.Peek(6)
	if err != nil {
		return err
	}

	filenameLen := int(uint16LE(lb[:2]))
	extraFieldLen := int(uint16LE(lb[2:4]))
	fileCommentLen := int(uint16LE(lb[4:]))
	contentLen := filenameLen + extraFieldLen + fileCommentLen

	// Discard the content (contentLen signature + contentLen)
	_, err = br.Discard(18 + contentLen)
	return err
}

// Discard the comment at the end of the central directory
func discardDirectoryEndRecord(br *bufio.Reader) error {
	// Discard directory end signature bytes of 20 bytes
	if _, err := br.Discard(20); err != nil {
		return err
	}
	commentLength, err := br.Peek(2)
	if err != nil {
		return err
	}

	_, err = br.Discard(2 + int(uint16LE(commentLength)))
	return err
}

// Discard Zip64 end of central directory locator
func discardDirectory64LocSignature(br *bufio.Reader) error {
	_, err := br.Discard(directory64LocLen)
	return err
}

// Discard Zip64 end of central directory record
func discardDirectory64EndRecord(br *bufio.Reader) error {
	lb, err := br.Peek(12)
	size := int(uint16LE(lb[4:12]))
	if err != nil {
		return err
	}

	_, err = br.Discard(size + 12)
	return err
}

// findDirectory64End tries to read the zip64 locator just before the
// directory end and returns the offset of the zip64 directory end if
// found.
func findDirectory64End(r io.ReaderAt, directoryEndOffset int64) (int64, error) {
	locOffset := directoryEndOffset - directory64LocLen
	if locOffset < 0 {
		return -1, nil // no need to look for a header outside the file
	}
	buf := make([]byte, directory64LocLen)
	if _, err := r.ReadAt(buf, locOffset); err != nil {
		return -1, err
	}
	b := readBuf(buf)
	if sig := b.uint32(); sig != directory64LocSignature {
		return -1, nil
	}
	b = b[4:]       // skip number of the disk with the start of the zip64 end of central directory
	p := b.uint64() // relative offset of the zip64 end of central directory record
	return int64(p), nil
}

// readDirectory64End reads the zip64 directory end and updates the
// directory end with the zip64 directory end values.
func readDirectory64End(r io.ReaderAt, offset int64, d *directoryEnd) (err error) {
	buf := make([]byte, directory64EndLen)
	
	if _, err := r.ReadAt(buf, offset); err != nil {
		return err
	}

	b := readBuf(buf)
	if sig := b.uint32(); sig != directory64EndSignature {
		return errors.New("zip: invalid format")
	}

	b = b[12:]                        // skip dir size, version and version needed (uint64 + 2x uint16)
	d.diskNbr = b.uint32()            // number of this disk
	d.dirDiskNbr = b.uint32()         // number of the disk with the start of the central directory
	d.dirRecordsThisDisk = b.uint64() // total number of entries in the central directory on this disk
	d.directoryRecords = b.uint64()   // total number of entries in the central directory
	d.directorySize = b.uint64()      // size of the central directory
	d.directoryOffset = b.uint64()    // offset of start of central directory with respect to the starting disk number

	return nil
}

// readDirectoryEnd tries to read EOCD to determine how many directory files file contains
func readDirectoryEnd(r io.ReaderAt, size int64, offset int64) (dir *directoryEnd, err error) {
	// look for directoryEndSignature in the last 1k, then in the last 65k
	var buf []byte
	var directoryEndOffset int64
	for i, bLen := range []int64{1024, 65 * 1024} {
		if bLen > size {
			bLen = size
		}
		buf = make([]byte, int(bLen))
		if _, err := r.ReadAt(buf, size-bLen); err != nil && err != io.EOF {
			return nil, err
		}
		if p := findSignatureInBlock(buf); p >= 0 {
			buf = buf[p:]
			directoryEndOffset = size - bLen + int64(p)
			break
		}
		if i == 1 || bLen == size {
			return nil, errors.New("zip: invalid format")
		}
	}

	// read header into struct
	b := readBuf(buf[4:]) // skip signature
	d := &directoryEnd{
		diskNbr:            uint32(b.uint16()),
		dirDiskNbr:         uint32(b.uint16()),
		dirRecordsThisDisk: uint64(b.uint16()),
		directoryRecords:   uint64(b.uint16()),
		directorySize:      uint64(b.uint32()),
		directoryOffset:    uint64(b.uint32()),
		commentLen:         b.uint16(),
	}
	l := int(d.commentLen)
	if l > len(b) {
		return nil, errors.New("zip: invalid comment length")
	}
	d.comment = string(b[:l])

	// These values mean that the file can be a zip64 file
	if d.directoryRecords == 0xffff || d.directorySize == 0xffffffff || d.directoryOffset == 0xffffffff {
		p, err := findDirectory64End(r, directoryEndOffset)
		if err == nil && p >= 0 {
			relativeOffset := p
			if offset < p {
				relativeOffset = p - offset
			}
			err = readDirectory64End(r, relativeOffset, d)
		}
		if err != nil {
			return nil, err
		}
	}

	if o := int64(d.directoryOffset); o < 0 {
		return nil, errors.New("zip: invalid format")
	}

	return d, nil
}

// readDirectoryHeader attempts to read directory header from reader
func readDirectoryHeader(r *bufio.Reader) (*FileHeader, error) {
	var buf [directoryHeaderLen]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return nil, err
	}
	b := readBuf(buf[:])
	if sig := b.uint32(); sig != directoryHeaderSignature {
		return nil, errors.New("Invalid format")
	}

	// Read bytes into memory in little endian order
	f := &FileHeader{}
	f.CreatorVersion = b.uint16()
	f.ReaderVersion = b.uint16()
	f.Flags = b.uint16()
	f.Method = b.uint16()
	f.ModifiedTime = b.uint16()
	f.ModifiedDate = b.uint16()
	f.CRC32 = b.uint32()
	f.CompressedSize = b.uint32()
	f.UncompressedSize = b.uint32()
	f.CompressedSize64 = uint64(f.CompressedSize)
	f.UncompressedSize64 = uint64(f.UncompressedSize)
	filenameLen := int(b.uint16())
	extraLen := int(b.uint16())
	commentLen := int(b.uint16())

	b = b[4:] // skipped start disk number and internal attributes (2x uint16)
	f.ExternalAttrs = b.uint32()
	f.headerOffset = int64(b.uint32())
	d := make([]byte, filenameLen+extraLen+commentLen)
	if _, err := io.ReadFull(r, d); err != nil {
		return nil, err
	}
	f.Name = string(d[:filenameLen])
	f.Extra = d[filenameLen : filenameLen+extraLen]
	f.Comment = string(d[filenameLen+extraLen:])
	// Determine the character encoding.
	utf8Valid1, utf8Require1 := DetectUTF8(f.Name)
	utf8Valid2, utf8Require2 := DetectUTF8(f.Comment)
	switch {
	case !utf8Valid1 || !utf8Valid2:
		// Name and Comment definitely not UTF-8.
		f.NonUTF8 = true
	case !utf8Require1 && !utf8Require2:
		// Name and Comment use only single-byte runes that overlap with UTF-8.
		f.NonUTF8 = false
	default:
		// Might be UTF-8, might be some other encoding; preserve existing flag.
		// Some ZIP writers use UTF-8 encoding without setting the UTF-8 flag.
		// Since it is impossible to always distinguish valid UTF-8 from some
		// other encoding (e.g., GBK or Shift-JIS), we trust the flag.
		f.NonUTF8 = f.Flags&0x800 == 0
	}


	if len(f.Extra) > 0 {
		b := readBuf(f.Extra)
		for len(b) >= 4 { // need at least tag and size
			tag := b.uint16()
			size := b.uint16()
			if int(size) > len(b) {
				return nil, errors.New("zip: invalid format")
			}
			eb := readBuf(b[:size])
			switch tag {
			case zip64ExtraId:
				// update directory values from the zip64 extra block
				if len(eb) >= 8 {
					f.UncompressedSize64 = eb.uint64()
				}
				if len(eb) >= 8 {
					f.CompressedSize64 = eb.uint64()
				}
				if len(eb) >= 8 {
					f.headerOffset = int64(eb.uint64())
				}
			}
			b = b[size:]
		}

		for _, v := range b {
			if v != 0 {
				return nil, errors.New("zip: invalid format")
			}
		}
	}

	return f, nil
}

func findSignatureInBlock(b []byte) int {
	for i := len(b) - directoryEndLen; i >= 0; i-- {
		// defined from directoryEndSignature in struct.go
		if b[i] == 'P' && b[i+1] == 'K' && b[i+2] == 0x05 && b[i+3] == 0x06 {
			// n is length of comment
			n := int(b[i+directoryEndLen-2]) | int(b[i+directoryEndLen-1])<<8
			if n+directoryEndLen+i <= len(b) {
				return i
			}
		}
	}
	return -1
}

func findSignature64InBlock(b []byte) int {
	for i := len(b) - directoryEndLen; i >= 0; i-- {
		// defined from directoryEndSignature in struct.go
		if b[i] == 'P' && b[i+1] == 'K' && b[i+2] == 0x06 && b[i+3] == 0x06 {
			// n is length of comment
			n := int(b[i+directoryEndLen-2]) | int(b[i+directoryEndLen-1])<<8
			if n+directoryEndLen+i <= len(b) {
				return i
			}
		}
	}
	return -1
}

func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReaderSize(r, bufferSize)}
}

// Next method advances to the next file in the archive and then it can be treated as an io.Reader to access the file's data.
// io.EOF is returned when the end of the zip has been reached.
// If Next is called again it will presume another zip file immediately follows and will advance to it.
func (r *Reader) Next() (*FileHeader, error) {
	if r.Reader != nil {
		if _, err := io.Copy(io.Discard, r.Reader); err != nil {
			return nil, err
		}
	}

	// Return signature bytes (first 4 bytes without moving the reader head)
	sigBytes, err := r.br.Peek(4)
	if err != nil {
		return nil, err
	}

	// Read signature read bytes
	sig := uint32LE(sigBytes)

	// Check if it contains file data or central directory
	switch sig {
	case fileHeaderDeflateSignature:
		break
	// Directory appears at the end of file (EOF)
	case directoryHeaderSignature:
		r.EOCDParsed = true
		return nil, discardCentralDirectory(r.br)
	default: // File is likely corrupted or in incorrect format
		return nil, errors.New("zip: invalid format")
	}

	// Buffer for file header
	headBuf := make([]byte, fileHeaderLen)
	if _, err := io.ReadFull(r.br, headBuf); err != nil {
		return nil, err
	}

	b := readBuf(headBuf[4:])
	// Read bytes into memory in little endian order
	f := &FileHeader{
		ReaderVersion:    b.uint16(),
		Flags:            b.uint16(),
		Method:           b.uint16(),
		ModifiedTime:     b.uint16(),
		ModifiedDate:     b.uint16(),
		CRC32:            b.uint32(),
		CompressedSize:   b.uint32(),
		UncompressedSize: b.uint32(),
	}

	filenameLen := b.uint16()
	extraLen := b.uint16()

	// Read filename and extra field info
	d := make([]byte, filenameLen+extraLen)
	if _, err := io.ReadFull(r.br, d); err != nil {
		return nil, err
	}

	// Detect character encoding of the filename
	nameBytes := d[:filenameLen]

	// Check if name is CodePage437 encoded or if it uses system local encoding.
	var convertedOutput []byte
	utf8Valid, utf8Require := DetectUTF8(string(nameBytes))
	if utf8Valid == false || utf8Require == false {
		// Decode WinZip encoding into utf-8
		conversion, err := DecodeWindows437(nameBytes)
		if err != nil {
			// If converting to Codepage437 fails
			convertedOutput = nameBytes
		}
		convertedOutput = conversion
	} else {
		convertedOutput = nameBytes
	}

	f.Name = string(convertedOutput)
	f.Extra = d[filenameLen : filenameLen+extraLen]

	compressedSize := uint64(f.CompressedSize)
	needUSize := f.UncompressedSize == ^uint32(0) // Need to calc 64-bit uncompressed size
	needCSize := f.CompressedSize == ^uint32(0)   // Need to calc 64-bit compressed size

	for extra := readBuf(f.Extra); len(extra) >= 4; { // need at least tag and size
		fieldTag := extra.uint16()
		fieldSize := int(extra.uint16())
		if len(extra) < fieldSize {
			break
		}
		fieldBuf := extra.sub(fieldSize)

		switch fieldTag {
		case zip64ExtraId:
			if needUSize {
				needUSize = false
				f.UncompressedSize64 = fieldBuf.uint64()
				f.UncompressedSize = uint32max
			}
			if needCSize {
				needCSize = false
				f.CompressedSize64 = fieldBuf.uint64()
				f.CompressedSize = uint32max
				compressedSize = f.CompressedSize64
			}
		}
	}

	// Check if file has datadescriptor by applying a mask
	// ie. if the bit at offset 3 (0x08) is set then the CRC-32 and file sizes
	// are not known when the header is written.
	// The fields in local header header are filled with zero and the CRC-32 and size are appended in
	// 12-byte structure (optionally preceeded by a 4-byte signature) immediately after the compressed data.

	if f.Flags&0x8 != 0 {
		r.Reader = &DescriptorReader{br: r.br, fileHeader: f}
	} else {
		// Read byte reader from current offset to the compressed size offset and decompress the data
		r.Reader = io.LimitReader(r.br, int64(compressedSize))
	}

	return f, nil
}

// DirectoryOffset reads EOCD record of the file and returns the offset to the EOCD record and size in bytes
func (r *Reader) DirectoryOffset(br io.ReaderAt, size int64, offset int64) (int64, int64, error) {
	end, err := readDirectoryEnd(br, size, offset)
	if err != nil {
		return 0, 0, err
	}
	return int64(end.directoryOffset), int64(end.directorySize), nil
}

// ReadAt methods allows reading buffer between two offsets
func (r *Reader) ReadAt(br io.ReaderAt, start int64, end int64) *io.SectionReader {
	return io.NewSectionReader(br, start, end)
}

// Stat method reads Central Directory record of the zip file and returns it's contents
// Stat makes possible to validate Zip contents or can be used to peek the structure before
// reading any actual files.
func (r *Reader) Stat(br *io.SectionReader) ([]*FileHeader, error) {
	buf := bufio.NewReader(br)
	files := make([]*FileHeader, 0)

	for {
		f, err := readDirectoryHeader(buf)
		if err != nil {
			break
		}
		files = append(files, f)
	}

	return files, nil
}


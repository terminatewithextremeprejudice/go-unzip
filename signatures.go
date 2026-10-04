package unzip

import (
	"archive/zip"
)

// Compression method
const (
	Store   uint16 = 0
	Deflate uint16 = 8
)

var sigBytes = []byte{0x50, 0x4b}

// Signatures
const (
	fileHeaderDeflateSignature = 0x4034b50
	fileHeaderEndSignature     = 0xFFFFFFFF
	directoryHeaderSignature   = 0x2014b50
	directoryEndSignature      = 0x06054b50
	directory64LocSignature    = 0x07064b50
	directory64EndSignature    = 0x6064b50
	dataDescriptorSignature    = 0x08074b50
	fileHeaderLen              = 30 // filename + extra
	directoryHeaderLen         = 46 // filename + extra + comment
	directoryEndLen            = 22 // + comment
	dataDescriptorLen          = 16 // four uint32: descriptor signature, crc32, compressed size, size
	dataDescriptor64Len        = 24 // descriptor with 8 byte sizes
	directory64LocLen          = 20
	directory64EndLen          = 56 // + extra
)

// Exports
const (
	DirectoryHeaderSignature = directoryHeaderSignature
	DirectoryEndSignature    = directoryEndSignature
	FileHeaderLen            = fileHeaderLen
)

// Zip versioning
const (
	zipVersion20 = 20 // 2.0
	zipVersion45 = 45 // 4.5 (zip64 archive)
)

// Limits for non zip64 files
const (
	uint16max = (1 << 16) - 1
	uint32max = (1 << 32) - 1
)

// Extra header id's
const (
	zip64ExtraId       = 0x0001 // zip64 Extended Information Extra Field
	ntfsExtraID        = 0x000a // NTFS
	unixExtraID        = 0x000d // UNIX
	extTimeExtraID     = 0x5455 // Extended timestamp
	infoZipUnixExtraID = 0x5855 // Info-ZIP Unix extensio
)

type directoryEnd struct {
	diskNbr            uint32 // unused
	dirDiskNbr         uint32 // unused
	dirRecordsThisDisk uint64 // unused
	directoryRecords   uint64
	directorySize      uint64
	directoryOffset    uint64 // relative to file
	commentLen         uint16
	comment            string
}

// FileHeader Exposes underlying zip FileHeader struct
type FileHeader zip.FileHeader

package unzip

// uint16LE reads an uint16 integer from a byte slice
func uint16LE(b []byte) uint16 {
	x := uint16(b[1]) << 8
	x |= uint16(b[0])
	return x
}

// uint32LE reads an uint32 integer from a byte slice
func uint32LE(b []byte) uint32 {
	x := uint32(b[3]) << 24
	x |= uint32(b[2]) << 16
	x |= uint32(b[1]) << 8
	x |= uint32(b[0])
	return x
}

// uint64LE converts the uint64 value stored as little endian to an uint64
// value.
func uint64LE(b []byte) uint64 {
	x := uint64(b[7]) << 56
	x |= uint64(b[6]) << 48
	x |= uint64(b[5]) << 40
	x |= uint64(b[4]) << 32
	x |= uint64(b[3]) << 24
	x |= uint64(b[2]) << 16
	x |= uint64(b[1]) << 8
	x |= uint64(b[0])
	return x
}

// putUint16LE puts an uint16 integer into a byte slice that must have at least
// a length of 2 bytes.
func putUint16LE(b []byte, x uint16) {
	b[0] = byte(x)
	b[1] = byte(x >> 8)
}

// putUint32LE puts an uint32 integer into a byte slice that must have at least
// a length of 4 bytes.
func putUint32LE(b []byte, x uint32) {
	b[0] = byte(x)
	b[1] = byte(x >> 8)
	b[2] = byte(x >> 16)
	b[3] = byte(x >> 24)
}

// putUint64LE puts the uint64 value into the byte slice as little endian
// value. The byte slice b must have at least place for 8 bytes.
func putUint64LE(b []byte, x uint64) {
	b[0] = byte(x)
	b[1] = byte(x >> 8)
	b[2] = byte(x >> 16)
	b[3] = byte(x >> 24)
	b[4] = byte(x >> 32)
	b[5] = byte(x >> 40)
	b[6] = byte(x >> 48)
	b[7] = byte(x >> 56)
}

func Uint16LEToByte(b []byte, x uint16) {
	putUint16LE(b, x)
}

func Uint32LEToByte(b []byte, x uint32) {
	putUint32LE(b, x)
}

func Uint64LEToByte(b []byte, x uint64) {
	putUint64LE(b, x)
}

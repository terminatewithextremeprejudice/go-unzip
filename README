# go-unzip

This library provides a memory-efficient method of decompressing Zip files in chunks without the need for disk I/O.

## Installation Instructions

Add [terminatewithextremeprejudice/go-zip](https://github.com/terminatewithextremeprejudice/go-unzip) as a dependency to your project:

```cli
go get github.com/terminatewithextremeprejudice/go-unzip
```

## Usage Examples

These examples demonstrate how to use [terminatewithextremeprejudice/go-unzip](https://github.com/terminatewithextremeprejudice/go-unzip) as a library.
The simplest way to use the library is to call the `NewReader` function which returns an `io.Reader`. Reader has a method Next which advances the readahead to the next file in archive. `io.EOF` will be returned when the and of archive is reached. 

```go
// Unpack the stream
zr := NewReader()
for {
	// read the headers preceding the actual body of the file
	headers, err := zr.Next();
	if err != nil {
		if err != io.EOF {
			// handle error
			log.Fatalf("Failed to unpack archive")
		}
	}

	// Decompressor function
	dcomp := GetDecompressor(headers.Method)
	// Read the body of the file, once we are done the reader will advance to the next file
	buff, err := io.ReadAll(dcomp(zr))
	....
}
```

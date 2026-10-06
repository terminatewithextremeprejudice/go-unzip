package unzip

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)


func TestReader(t *testing.T) {
	t.Run("should read the zip file", func(t *testing.T) {
		// Create a buffer to write our archive to.
		var (
			buf = new(bytes.Buffer)
		)

		// Create a new zip archive.
		w := zip.NewWriter(buf)
		// Add some files to the archive.
		var files = []struct {
			Name, Body string
		}{
			{"raaaa.txt", "Hello world!! This is file that contains compressed data."},
			{"raaaa2.txt", "Hello world!! This is another that file contains compressed data."},
		}
		for _, file := range files {
			f, err := w.Create(file.Name)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Write([]byte(file.Body))
			if err != nil {
				t.Fatal(err)
			}
		}

		// Make sure to check the error on Close.
		err := w.Close()
		if err != nil {
			t.Fatal(err)
		}

		zr := NewReader(buf)
		i := 0
		for {
			headers, err := zr.Next()
			if err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}
				break
			}

			dcomp := decompressor(headers.Method)
			returnedBuff, _ := io.ReadAll(dcomp(zr))
			assert.Equal(t, headers.Name, files[i].Name)
			assert.Equal(t, string(returnedBuff), files[i].Body)

			i++
		}
	})

	t.Run("should convert invalid encoding on filename into utf-8 when decompressed", func(t *testing.T) {
		var testData = []struct {
			utf8, encoding, body string
		}{
			{"Héllô ¥º⌠£╛", "H\x82ll\x93 \x9d\xa7\xf4\x9c\xbe", "Lorem ipsum"},
		}

		for _, file := range testData {
			// Create a buffer to write our archive to.
			var (
				buf = new(bytes.Buffer)
			)

			// Create a new zip archive.
			w := zip.NewWriter(buf)
			f, err := w.Create(file.encoding)
			if err != nil {
				t.Fatal(err)
			}

			_, err = f.Write([]byte(file.body))
			if err != nil {
				t.Fatal(err)
			}

			// Make sure to check the error on Close.
			err = w.Close()
			if err != nil {
				t.Fatal(err)
			}

			zr := NewReader(buf)
			for {
				headers, err := zr.Next()
				if err != nil {
					break
				}

				assert.Equal(t, headers.Name, file.utf8)
			}
		}
	})

	t.Run("calculates crc32 from the content and it should not match with invalid crc of the corrupted zip file", func(t *testing.T) {
		file, _ := os.Open("./test_data/invalid_crc.zip")
		zr := NewReader(file)
		defer file.Close()

		for {
			headers, err := zr.Next()
			if err != nil {
				if err != io.EOF {

				}

				break
			}

			// Validate CRC32 Signature
			crc := &CRCReader{
				Hash: crc32.NewIEEE(),
				Crc:  &headers.CRC32,
			}

			// Decompress data if file header has compression flag set
			dcomp := decompressor(headers.Method)
			// If has no data descriptor, then crc is set in headers
			if headers.Flags&0x8 == 0 {
				crc.Crc = &headers.CRC32
			}
			crc.Reader = dcomp(zr)

			// Copy reader to temporary buffer to not corrupt the original reader
			buf := new(bytes.Buffer)
			io.Copy(buf, crc)

			assert.NotEqual(t, crc.Hash.Sum32(), *crc.Crc)
		}
	})

	t.Run("should fail uncompressing the data and stop gracefully if compressed data has been corrupted", func(t *testing.T) {
		file, _ := os.Open("./test_data/corrupted_deflate.zip")
		zr := NewReader(file)
		defer file.Close()

		for {
			headers, err := zr.Next()
			if err != nil {
				if err != io.EOF {

				}

				break
			}

			// Validate CRC32 Signature
			crc := &CRCReader{
				Hash: crc32.NewIEEE(),
				Crc:  &headers.CRC32,
			}

			// Decompress data if file header has compression flag set
			dcomp := decompressor(8)
			// If has no data descriptor, then crc is set in headers
			if headers.Flags&0x8 == 0 {
				crc.Crc = &headers.CRC32
			}
			crc.Reader = dcomp(zr)
			// Write content temporarily buffer
			io.ReadAll(crc)

			assert.NotEqual(t, crc.Hash.Sum32(), *crc.Crc)
		}
	})

	t.Run("should read directory structure in different compressions and validate crc32 of the content", func(t *testing.T) {
		var files = []struct {
			Name        string
			Compression uint16
			Body        string
		}{
			{"a", Deflate, "This archive contains something compressed."},
			{"b", Store, "This archive contains something uncompressed."},
		}

		tmpDir, _ := os.MkdirTemp("", "")
		targetDir, _ := os.MkdirTemp("", "")
		source := tmpDir
		target := targetDir
		os.MkdirAll(tmpDir, 0777)
		os.MkdirAll(target, 0777)

		for _, file := range files {
			z, _ := os.Create(tmpDir + "/" + file.Name)
			z.Write([]byte(file.Body))
			z.Close()
		}

		baseDir := filepath.Base(source)

		defer os.RemoveAll(tmpDir)
		defer os.RemoveAll(targetDir)

		zipfile, err := os.Create(source + ".zip")
		if err != nil {
			t.Fatal(err)
		}

		// Create a new zip archive.
		archive := zip.NewWriter(zipfile)

		x := 0
		filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if info.IsDir() {
				header.Name += "/"
			} else {
				r := files[x]
				x = x + 1
				header.Method = r.Compression
			}

			writer, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			return err
		})

		archive.Close()
		defer zipfile.Close()

		buf, _ := os.Open(source + ".zip")
		zr := NewReader(buf)

		i := 0
		for {
			headers, err := zr.Next()
			if err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}

				break
			}

			if i == 0 {
				assert.Equal(t, headers.Name, baseDir+"/")
				assert.Equal(t, headers.FileInfo().IsDir(), true)
				i++
				continue
			}

			// Validate CRC32 Signature
			crc := &CRCReader{
				Hash: crc32.NewIEEE(),
				Crc:  &headers.CRC32,
			}

			// Decompress data if file header has compression flag set
			dcomp := decompressor(headers.Method)
			// If has no data descriptor, then crc is set in headers
			if headers.Flags&0x8 == 0 {
				crc.Crc = &headers.CRC32
			}
			crc.Reader = dcomp(zr)
			// Write content temporarily buffer
			returnedBytes, _ := io.ReadAll(crc)

			assert.Equal(t, headers.Name, baseDir+"/"+files[i-1].Name)
			assert.Equal(t, headers.FileInfo().IsDir(), false)
			assert.Equal(t, string(returnedBytes), files[i-1].Body)
			assert.Equal(t, *crc.Crc, crc.Hash.Sum32())

			i++
		}
	})
	
	t.Run("should be able to read ZIP64 file format", func(t *testing.T) {
		// Create archive bigger than 4G
		archive, err := os.Create("z.zip")
		if err != nil {
			panic(err)
		}
		defer archive.Close()
		defer os.Remove("z.zip")
		
		zipWriter := zip.NewWriter(archive)

		comment := strings.Repeat("1", 64<<10-1)
		for i := 0; i < 65428; i++ {
			name := fmt.Sprintf("%060x.txt", i)
			h := &zip.FileHeader{
				Name:    name,
				// Comment: comment,
			}
			fileWriter, err := zipWriter.CreateHeader(h)
			if err != nil {
				panic(err)
			}

			fileWriter.Write([]byte(comment))
		}

		zipWriter.Close()
		archive, _ = os.Open("z.zip")
		zr := NewReader(archive)

		i := 0
		for {
			headers, err := zr.Next()
			if err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}

				break
			}
			// Validate CRC32 Signature
			crc := &CRCReader{
				Hash: crc32.NewIEEE(),
				Crc:  &headers.CRC32,
			}

			// Decompress data if file header has compression flag set
			dcomp := decompressor(headers.Method)
			// If has no data descriptor, then crc is set in headers
			if headers.Flags&0x8 == 0 {
				crc.Crc = &headers.CRC32
			}
			crc.Reader = dcomp(zr)
			// Write content temporarily buffer
			returnedBytes, _ := io.ReadAll(crc)

			assert.Equal(t, len(returnedBytes), 0xffff)

			i++
		}

	})

	t.Run("should be able to read central directory record from part written by a zip writer", func(t *testing.T) {
		var (
			buf = new(bytes.Buffer)
		)

		// Create a new zip archive.
		w := zip.NewWriter(buf)
		// Add some files to the archive.
		var files = []struct {
			Name, Body string
		}{
			{"raaaa.txt", "Hello world!! This is file that contains compressed data."},
			{"raaaa2.txt", "Hello world!! This is another that file contains compressed data."},
		}
		for _, file := range files {
			f, err := w.Create(file.Name)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Write([]byte(file.Body))
			if err != nil {
				t.Fatal(err)
			}
		}

		// Make sure to check the error on Close.
		err := w.Close()
		if err != nil {
			t.Fatal(err)
		}

		// Read bytes from offset to skip parts of single files
		reader := bytes.NewReader(buf.Bytes()[100:])
		zr := NewReader(buf)
		offset, size, _ := zr.DirectoryOffset(reader, int64(buf.Len()), 0)
		reader2 := zr.ReadAt(bytes.NewReader(buf.Bytes()[offset:offset+size]), 0, size)
		headers, _ := zr.Stat(reader2)

		i := 0
		for _, header := range headers {
			assert.Equal(t, header.Name, files[i].Name)
			i++
		}
	})

	t.Run("should be able to read central directory record from zip file", func(t *testing.T) {
		var (
			buf = new(bytes.Buffer)
		)

		f, _ := os.Open("./test_data/single_file.zip")
		buf.ReadFrom(f)

		// Read bytes from offset to skip parts of single files
		reader := bytes.NewReader(buf.Bytes())
		zr := NewReader(buf)
		offset, size, _ := zr.DirectoryOffset(reader, int64(buf.Len()), 0)
		reader2 := zr.ReadAt(bytes.NewReader(buf.Bytes()[offset:offset+size]), 0, size)
		headers, _ := zr.Stat(reader2)

		i := 0
		for _, header := range headers {
			assert.Equal(t, header.Name, "helloworld.txt")
			i++
		}
	})

	t.Run("should be able to read central directory record from zip64 file", func(t *testing.T) {
		archive, err := os.Create("z.zip")
		if err != nil {
			panic(err)
		}
		defer archive.Close()
		defer os.Remove(archive.Name())
		
		zipWriter := zip.NewWriter(archive)
		//defer zipWriter.Close()

		comment := strings.Repeat("1", 64<<10-1)
		for i := 0; i < 65428; i++ {
			name := fmt.Sprintf("%060x.txt", i)
			h := &zip.FileHeader{
				Name:    name,
				// Comment: comment,
			}
			fileWriter, err := zipWriter.CreateHeader(h)
			if err != nil {
				panic(err)
			}

			fileWriter.Write([]byte(comment))
		}


		zipWriter.Close()
		buffer := make([]byte, 65535*2)

		f, err := os.Open("z.zip")
		if err != nil {
			t.Fatalf("invalid zip archive")
		}
		fileStats, _ := f.Stat()

		_, err = f.Seek(-65535*2, io.SeekEnd)
		if err != nil {
			t.Fatalf("invalid zip archive")
		}

		_, err = f.Read(buffer)
		if err != nil && err != io.EOF {
			t.Fatalf("invalid zip archive")
		}

		reader := bytes.NewReader(buffer)
		zr := NewReader(reader)
		offset, size, err := zr.DirectoryOffset(reader, int64(len(buffer)), fileStats.Size()-65535*2)
		if err != nil {
			t.Fatalf("invalid zip archive")
		}

		assert.Greater(t, offset, int64(0))	
		assert.Greater(t, size, int64(0))	
	})

	t.Run("should be able to read a zip file even if the data is not instantly fully available", func(t *testing.T) {

	
		var (
			files = []struct {
				Name, Body string
			}{
				{"raaaa.txt", "Hello world!! This is file that contains compressed data."},
				{"raaaa2.txt", "Hello world!! This is another that file contains compressed data."},
			}
			buf = new(bytes.Buffer)
			wg sync.WaitGroup
		)

		// Create a new zip archive.
		zw := zip.NewWriter(buf)

		// Add some files to the archive.
		for _, file := range files {
			f, err := zw.Create(file.Name)
			if err != nil {
				t.Fatal(err)
			}

			f.Write([]byte(file.Body))
		}

		zw.Close()
		
		r, w := io.Pipe()
		zr := NewReader(r)

		wg.Add(1)
		go func() (err error) {
			defer wg.Done()
			defer w.Close()

			l := len(buf.Bytes())

			for i := 0; i < l; i = i+4 {
				end := min(i+4, l)
				w.Write([]byte(buf.Bytes()[i:end]))
				time.Sleep(10 * time.Millisecond)
			}
			return nil
		}()

	
		i := 0
		for {
			headers, err := zr.Next()
			if err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}
				break
			}

			dcomp := decompressor(headers.Method)
			returnedBuff, _ := io.ReadAll(dcomp(zr))
			assert.Equal(t, headers.Name, files[i].Name)
			assert.Equal(t, string(returnedBuff), files[i].Body)

			i++
		}


		wg.Wait()
	})
}

type zeros struct{}

func (zeros) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = 0
	}
	return len(b), nil
}

func makeZip(name string, r io.Reader) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	wf, _ := w.Create(name)
	io.Copy(wf, r)
	w.Close()
	return buf.Bytes()
}

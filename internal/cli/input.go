package cli

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"unicode/utf16"
)

// Byte-order marks Windows PowerShell 5.1 writes: UTF-16LE for Out-File and >
// by default, UTF-8 with -Encoding utf8 and ahead of text piped to a command.
var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// readInput reads a command's JSON input: the file at path, or stdin for "-".
// encoding/json takes only BOM-less UTF-8, so a UTF-8 BOM is dropped and UTF-16
// with a BOM is decoded to UTF-8; input without a BOM is returned as read.
func readInput(path string, stdin io.Reader) ([]byte, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(b, utf8BOM):
		return b[len(utf8BOM):], nil
	case bytes.HasPrefix(b, utf16LEBOM):
		return decodeUTF16(b[len(utf16LEBOM):], binary.LittleEndian)
	case bytes.HasPrefix(b, utf16BEBOM):
		return decodeUTF16(b[len(utf16BEBOM):], binary.BigEndian)
	}
	return b, nil
}

// decodeUTF16 turns UTF-16 code units in the given byte order into UTF-8.
func decodeUTF16(b []byte, order binary.ByteOrder) ([]byte, error) {
	if len(b)%2 != 0 {
		return nil, errors.New("UTF-16 input with an odd number of bytes")
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = order.Uint16(b[2*i:])
	}
	return []byte(string(utf16.Decode(units))), nil
}

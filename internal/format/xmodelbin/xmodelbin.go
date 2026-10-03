// Package xmodelbin reads the material names an xmodel's LOD file uses. The
// linker takes a model's materials from that file, not from its GDT: APE copies
// them into the xmodel's "materials" field, but that field is empty for most
// models and can be stale, so the file is the only reliable source.
//
// Two encodings are read:
//   - .xmodel_bin: "*LZ4*", the uncompressed size (uint32, little-endian), one
//     LZ4 block. Decompressed, it is a stream of tokens aligned to 4 bytes, each
//     starting with a 16-bit id: NUMMATERIALS (0xA1B2, then the count) and, for
//     each material, MATERIAL (0xA700, then its index and its NUL-terminated
//     name, padded to 4 bytes).
//   - .xmodel_export, the text form: MATERIAL <index> "<name>" … lines. Some
//     tools write it LZ4-compressed under the .xmodel_bin name; both are handled.
package xmodelbin

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

const (
	tokNumMaterials = 0xA1B2
	tokMaterial     = 0xA700
	maxSize         = 1 << 30 // a declared size beyond this is a corrupt header, not a model
)

// Materials returns the names of a model file's materials, in index order.
func Materials(b []byte) ([]string, error) {
	if rest, ok := bytes.CutPrefix(b, []byte("*LZ4*")); ok {
		if len(rest) < 4 {
			return nil, errors.New("xmodel_bin: truncated header")
		}
		n := binary.LittleEndian.Uint32(rest)
		if n > maxSize {
			return nil, fmt.Errorf("xmodel_bin: declared size %d is not plausible", n)
		}
		d, err := lz4Block(rest[4:], int(n))
		if err != nil {
			return nil, fmt.Errorf("xmodel_bin: %w", err)
		}
		b = d
	}
	if isText(b) {
		return textMaterials(b)
	}
	return tokenMaterials(b)
}

// tokenMaterials reads the binary token stream. The material section is at the
// end of the stream, so the NUMMATERIALS candidates are tried from the last
// one back: a float elsewhere can hold the same 16 bits, but not one followed
// by count well-formed MATERIAL entries.
func tokenMaterials(d []byte) ([]string, error) {
	for i := (len(d) - 4) &^ 3; i >= 0; i -= 4 {
		if binary.LittleEndian.Uint16(d[i:]) != tokNumMaterials {
			continue
		}
		count := int(binary.LittleEndian.Uint16(d[i+2:]))
		if names, ok := materialEntries(d, i+4, count); ok {
			return names, nil
		}
	}
	return nil, errors.New("xmodel_bin: no material section")
}

// materialEntries reads count MATERIAL tokens, indexed 0..count-1, from start;
// property tokens between them are skipped word by word.
func materialEntries(d []byte, start, count int) ([]string, bool) {
	names := make([]string, 0, count)
	i := start
	for k := 0; k < count; k++ {
		for ; i+4 <= len(d); i += 4 {
			if binary.LittleEndian.Uint16(d[i:]) == tokMaterial && int(binary.LittleEndian.Uint16(d[i+2:])) == k {
				break
			}
		}
		if i+4 > len(d) {
			return nil, false
		}
		name, next, ok := cString(d, i+4)
		if !ok {
			return nil, false
		}
		names = append(names, name)
		i = next
	}
	return names, true
}

// cString reads a NUL-terminated printable name at i and returns the next
// 4-byte-aligned offset after it.
func cString(d []byte, i int) (string, int, bool) {
	end := bytes.IndexByte(d[i:], 0)
	if end <= 0 {
		return "", 0, false
	}
	s := d[i : i+end]
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return "", 0, false
		}
	}
	return string(s), (i + end + 1 + 3) &^ 3, true
}

var materialLine = regexp.MustCompile(`^MATERIAL\s+(\d+)\s+"([^"]*)"`)

// textMaterials reads the MATERIAL lines of an .xmodel_export.
func textMaterials(b []byte) ([]string, error) {
	var names []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		if m := materialLine.FindSubmatch(sc.Bytes()); m != nil {
			if k, _ := strconv.Atoi(string(m[1])); k == len(names) {
				names = append(names, string(m[2]))
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("xmodel_export: %w", err)
	}
	return names, nil
}

// isText reports whether a model file is the text export (it starts with a
// comment or the MODEL keyword, after optional whitespace).
func isText(b []byte) bool {
	t := bytes.TrimLeft(b, " \t\r\n")
	return bytes.HasPrefix(t, []byte("//")) || bytes.HasPrefix(t, []byte("MODEL"))
}

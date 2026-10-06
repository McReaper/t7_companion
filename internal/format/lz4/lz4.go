// Package lz4 unpacks the compression the BO3 export tools write model and
// animation files with (.xmodel_bin, .xanim_bin): "*LZ4*", the uncompressed
// size (uint32, little-endian), then one raw LZ4 block, without a frame.
package lz4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// maxSize: a declared size beyond this is a corrupt header, not a file.
const maxSize = 1 << 30

var errCorrupt = errors.New("corrupt LZ4 block")

// Unpack returns an export file's contents: decompressed when it starts with
// "*LZ4*", as is otherwise.
func Unpack(b []byte) ([]byte, error) {
	rest, ok := bytes.CutPrefix(b, []byte("*LZ4*"))
	if !ok {
		return b, nil
	}
	if len(rest) < 4 {
		return nil, errors.New("truncated *LZ4* header")
	}
	n := binary.LittleEndian.Uint32(rest)
	if n > maxSize {
		return nil, fmt.Errorf("declared size %d is not plausible", n)
	}
	return Block(rest[4:], int(n))
}

// Block decodes one raw LZ4 block (no frame) into exactly n bytes. Each
// sequence is a token (literal length, match length), the literals, then a
// 2-byte back-reference offset; lengths of 15 continue in 255-steps.
func Block(src []byte, n int) ([]byte, error) {
	dst := make([]byte, 0, n)
	for i := 0; i < len(src); {
		tok := src[i]
		i++
		lit, ok := length(src, &i, int(tok>>4))
		if !ok || i+lit > len(src) || len(dst)+lit > n {
			return nil, errCorrupt
		}
		dst = append(dst, src[i:i+lit]...)
		i += lit
		if i == len(src) || len(dst) == n {
			break // the last sequence has literals only
		}
		if i+2 > len(src) {
			return nil, errCorrupt
		}
		off := int(src[i]) | int(src[i+1])<<8
		i += 2
		ml, ok := length(src, &i, int(tok&15))
		ml += 4
		if !ok || off == 0 || off > len(dst) || len(dst)+ml > n {
			return nil, errCorrupt
		}
		for k := 0; k < ml; k++ { // byte by byte: a match may overlap its own output
			dst = append(dst, dst[len(dst)-off])
		}
	}
	if len(dst) != n {
		return nil, errCorrupt
	}
	return dst, nil
}

// length reads a 4-bit length and, when it is 15, its 255-step continuation.
func length(src []byte, i *int, v int) (int, bool) {
	if v != 15 {
		return v, true
	}
	for {
		if *i >= len(src) {
			return 0, false
		}
		b := src[*i]
		*i++
		v += int(b)
		if b != 255 {
			return v, true
		}
	}
}

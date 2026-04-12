package shard

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"

	"github.com/SCKelemen/codesearch/store"
)

const (
	// Magic identifies a codesearch shard file.
	Magic = "CSHR"
	// Version is the current shard format version.
	Version uint32 = 1
)

var (
	magicBytes          = []byte(Magic)
	errChecksumMismatch = errors.New("shard checksum mismatch")
)

// File layout:
//   - 4-byte magic: "CSHR"
//   - 4-byte version: uint32 little endian
//   - 4-byte header length: uint32 little endian
//   - JSON header payload
//   - section payloads referenced by the header offsets table
//   - 4-byte CRC32 footer over all preceding bytes
//
// The header stores ShardMeta plus absolute offsets for documents, trigrams,
// vectors, and symbols. Vectors and symbols are optional.
type fileHeader struct {
	Meta     store.ShardMeta `json:"meta"`
	Sections sectionTable    `json:"sections"`
}

type sectionTable struct {
	Documents sectionRef `json:"documents"`
	Trigrams  sectionRef `json:"trigrams"`
	Vectors   sectionRef `json:"vectors,omitempty"`
	Symbols   sectionRef `json:"symbols,omitempty"`
}

type sectionRef struct {
	Offset uint64 `json:"offset,omitempty"`
	Length uint64 `json:"length,omitempty"`
}

func (s sectionRef) present() bool {
	return s.Length > 0
}

func encodeShard(meta store.ShardMeta, docs []store.Document, trigrams []store.PostingList, vectors []store.StoredVector, symbols []store.Symbol, refs []store.Reference) ([]byte, error) {
	docBytes, err := json.Marshal(docs)
	if err != nil {
		return nil, fmt.Errorf("marshal documents: %w", err)
	}
	trigramBytes, err := json.Marshal(trigrams)
	if err != nil {
		return nil, fmt.Errorf("marshal trigrams: %w", err)
	}
	vectorBytes, err := json.Marshal(vectors)
	if err != nil {
		return nil, fmt.Errorf("marshal vectors: %w", err)
	}
	symbolBytes, err := json.Marshal(struct {
		Symbols    []store.Symbol    `json:"symbols"`
		References []store.Reference `json:"references"`
	}{Symbols: symbols, References: refs})
	if err != nil {
		return nil, fmt.Errorf("marshal symbols: %w", err)
	}

	payloads := [][]byte{docBytes, trigramBytes, nil, nil}
	if len(vectors) > 0 {
		payloads[2] = vectorBytes
	}
	if len(symbols) > 0 || len(refs) > 0 {
		payloads[3] = symbolBytes
	}
	var headerBytes []byte
	for range 8 {
		offset := uint64(12 + len(headerBytes))
		header := fileHeader{Meta: meta}
		header.Sections.Documents = sectionRef{Offset: offset, Length: uint64(len(payloads[0]))}
		offset += uint64(len(payloads[0]))
		header.Sections.Trigrams = sectionRef{Offset: offset, Length: uint64(len(payloads[1]))}
		offset += uint64(len(payloads[1]))
		if len(payloads[2]) > 0 {
			header.Sections.Vectors = sectionRef{Offset: offset, Length: uint64(len(payloads[2]))}
			offset += uint64(len(payloads[2]))
		}
		if len(payloads[3]) > 0 {
			header.Sections.Symbols = sectionRef{Offset: offset, Length: uint64(len(payloads[3]))}
		}
		next, err := json.Marshal(header)
		if err != nil {
			return nil, fmt.Errorf("marshal header: %w", err)
		}
		if bytes.Equal(next, headerBytes) {
			headerBytes = next
			break
		}
		headerBytes = next
	}

	buf := bytes.NewBuffer(make([]byte, 0, 12+len(headerBytes)+len(payloads[0])+len(payloads[1])+len(payloads[2])+len(payloads[3])+4))
	buf.Write(magicBytes)
	if err := binary.Write(buf, binary.LittleEndian, Version); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(headerBytes))); err != nil {
		return nil, err
	}
	buf.Write(headerBytes)
	for _, payload := range payloads {
		if len(payload) > 0 {
			buf.Write(payload)
		}
	}
	checksum := crc32.ChecksumIEEE(buf.Bytes())
	if err := binary.Write(buf, binary.LittleEndian, checksum); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeHeader(raw []byte) (fileHeader, error) {
	var header fileHeader
	if len(raw) < 16 {
		return header, fmt.Errorf("shard too small")
	}
	if !bytes.Equal(raw[:4], magicBytes) {
		return header, fmt.Errorf("invalid magic %q", raw[:4])
	}
	if version := binary.LittleEndian.Uint32(raw[4:8]); version != Version {
		return header, fmt.Errorf("unsupported shard version %d", version)
	}
	headerLen := int(binary.LittleEndian.Uint32(raw[8:12]))
	footerStart := len(raw) - 4
	if 12+headerLen > footerStart {
		return header, fmt.Errorf("invalid header length %d", headerLen)
	}
	if crc32.ChecksumIEEE(raw[:footerStart]) != binary.LittleEndian.Uint32(raw[footerStart:]) {
		return header, errChecksumMismatch
	}
	if err := json.Unmarshal(raw[12:12+headerLen], &header); err != nil {
		return header, fmt.Errorf("decode header: %w", err)
	}
	for _, ref := range []sectionRef{header.Sections.Documents, header.Sections.Trigrams, header.Sections.Vectors, header.Sections.Symbols} {
		if !ref.present() {
			continue
		}
		end := ref.Offset + ref.Length
		if ref.Offset < uint64(12+headerLen) || end > uint64(footerStart) || end < ref.Offset {
			return header, fmt.Errorf("section out of bounds offset=%d length=%d", ref.Offset, ref.Length)
		}
	}
	return header, nil
}

func sectionBytes(raw []byte, ref sectionRef) []byte {
	if !ref.present() {
		return nil
	}
	return raw[ref.Offset : ref.Offset+ref.Length]
}

package telegram

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	webLocationFlag   = 1 << 24
	fileReferenceFlag = 1 << 25
)

// DecodedFileID contains unpacked MTProto parameters from a Telegram file_id.
type DecodedFileID struct {
	Major         byte
	Minor         byte
	FileType      int32
	DCID          int32
	MediaID       int64
	AccessHash    int64
	FileReference []byte
}

// DecodeFileID parses a Pyrogram/TDLib encoded file_id string.
func DecodeFileID(fileID string) (*DecodedFileID, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, errors.New("empty file_id")
	}

	// 1. Base64 URL decode (handle unpadded base64)
	raw, err := base64.RawURLEncoding.DecodeString(fileID)
	if err != nil {
		// Fallback to standard URL encoding
		raw, err = base64.URLEncoding.DecodeString(fileID)
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}
	}

	// 2. Zero-value RLE decode
	decoded := rleDecode(raw)
	if len(decoded) < 10 {
		return nil, errors.New("decoded file_id is too short")
	}

	// 3. Read version
	major := decoded[len(decoded)-1]
	minor := byte(0)
	payload := decoded[:len(decoded)-1]
	if major >= 4 {
		minor = decoded[len(decoded)-2]
		payload = decoded[:len(decoded)-2]
	}

	buf := bytes.NewReader(payload)

	var fileType, dcID int32
	if err := binary.Read(buf, binary.LittleEndian, &fileType); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &dcID); err != nil {
		return nil, err
	}

	hasFileReference := (fileType & fileReferenceFlag) != 0
	fileType = fileType &^ webLocationFlag &^ fileReferenceFlag

	var fileReference []byte
	if hasFileReference {
		ref, err := readTLBytes(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read file_reference: %w", err)
		}
		fileReference = ref
	}

	var mediaID, accessHash int64
	if err := binary.Read(buf, binary.LittleEndian, &mediaID); err != nil {
		return nil, fmt.Errorf("failed to read mediaID: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &accessHash); err != nil {
		return nil, fmt.Errorf("failed to read accessHash: %w", err)
	}

	return &DecodedFileID{
		Major:         major,
		Minor:         minor,
		FileType:      fileType,
		DCID:          dcID,
		MediaID:       mediaID,
		AccessHash:    accessHash,
		FileReference: fileReference,
	}, nil
}

func rleDecode(src []byte) []byte {
	var dst []byte
	zero := false

	for _, b := range src {
		if b == 0 {
			zero = true
			continue
		}
		if zero {
			dst = append(dst, bytes.Repeat([]byte{0}, int(b))...)
			zero = false
		} else {
			dst = append(dst, b)
		}
	}
	return dst
}

func readTLBytes(r io.Reader) ([]byte, error) {
	var firstByte [1]byte
	if _, err := io.ReadFull(r, firstByte[:]); err != nil {
		return nil, err
	}

	var length int
	var padding int

	if firstByte[0] < 254 {
		length = int(firstByte[0])
		padding = (4 - ((length + 1) % 4)) % 4
	} else {
		var lenBytes [3]byte
		if _, err := io.ReadFull(r, lenBytes[:]); err != nil {
			return nil, err
		}
		length = int(lenBytes[0]) | int(lenBytes[1])<<8 | int(lenBytes[2])<<16
		padding = (4 - (length % 4)) % 4
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	if padding > 0 {
		pad := make([]byte, padding)
		if _, err := io.ReadFull(r, pad); err != nil {
			return nil, err
		}
	}

	return data, nil
}

// EncodeFileUniqueID encodes a document media_id into Telegram's Bot API/TDLib file_unique_id string.
func EncodeFileUniqueID(mediaID int64) string {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, int32(2)) // FileUniqueType.DOCUMENT = 2
	_ = binary.Write(buf, binary.LittleEndian, mediaID)
	rle := rleEncode(buf.Bytes())
	return base64.RawURLEncoding.EncodeToString(rle)
}

// EncodeFileID creates a Pyrogram/Bot API compatible file_id string for an audio document.
func EncodeFileID(mediaID, accessHash int64, dcID int32, fileReference []byte) string {
	buf := new(bytes.Buffer)
	fileType := int32(9) // FileType.AUDIO = 9
	if len(fileReference) > 0 {
		fileType |= fileReferenceFlag // (1 << 25)
	}

	_ = binary.Write(buf, binary.LittleEndian, fileType)
	_ = binary.Write(buf, binary.LittleEndian, dcID)

	if len(fileReference) > 0 {
		buf.Write(writeTLBytes(fileReference))
	}

	_ = binary.Write(buf, binary.LittleEndian, mediaID)
	_ = binary.Write(buf, binary.LittleEndian, accessHash)

	// minor=30, major=4
	buf.WriteByte(30)
	buf.WriteByte(4)

	rle := rleEncode(buf.Bytes())
	return base64.RawURLEncoding.EncodeToString(rle)
}

func rleEncode(src []byte) []byte {
	var dst []byte
	n := byte(0)

	for _, b := range src {
		if b == 0 {
			n++
			if n == 255 {
				dst = append(dst, 0, n)
				n = 0
			}
		} else {
			if n > 0 {
				dst = append(dst, 0, n)
				n = 0
			}
			dst = append(dst, b)
		}
	}
	if n > 0 {
		dst = append(dst, 0, n)
	}
	return dst
}

func writeTLBytes(data []byte) []byte {
	buf := new(bytes.Buffer)
	l := len(data)
	if l < 254 {
		buf.WriteByte(byte(l))
		buf.Write(data)
		pad := (4 - ((l + 1) % 4)) % 4
		for i := 0; i < pad; i++ {
			buf.WriteByte(0)
		}
	} else {
		buf.WriteByte(254)
		buf.WriteByte(byte(l & 0xff))
		buf.WriteByte(byte((l >> 8) & 0xff))
		buf.WriteByte(byte((l >> 16) & 0xff))
		buf.Write(data)
		pad := (4 - (l % 4)) % 4
		for i := 0; i < pad; i++ {
			buf.WriteByte(0)
		}
	}
	return buf.Bytes()
}

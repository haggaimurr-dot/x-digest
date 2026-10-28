package main

import (
	"bytes"
	"io"
	"mime/multipart"
)

// Upload content check (2026-10-28): the extension allow-list alone lets a
// renamed .exe through, so the first bytes must match the claimed type.
// Office OOXML files are zip archives, the legacy ones OLE2 compound files.
// Text formats only need to be free of NUL bytes (and not an executable).

var (
	magicZip = []byte("PK\x03\x04")
	magicOLE = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

func fileHeadMatches(fh *multipart.FileHeader, ext string) bool {
	f, err := fh.Open()
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 1024)
	n, _ := io.ReadFull(f, head)
	return headMatches(head[:n], ext)
}

func headMatches(h []byte, ext string) bool {
	switch ext {
	case "pdf":
		return bytes.HasPrefix(h, []byte("%PDF-"))
	case "docx", "xlsx", "pptx":
		return bytes.HasPrefix(h, magicZip)
	case "doc", "xls", "ppt":
		return bytes.HasPrefix(h, magicOLE)
	case "png":
		return bytes.HasPrefix(h, []byte("\x89PNG\r\n\x1a\n"))
	case "jpg", "jpeg":
		return bytes.HasPrefix(h, []byte{0xFF, 0xD8, 0xFF})
	case "gif":
		return bytes.HasPrefix(h, []byte("GIF87a")) || bytes.HasPrefix(h, []byte("GIF89a"))
	case "webp":
		return len(h) >= 12 && bytes.Equal(h[:4], []byte("RIFF")) && bytes.Equal(h[8:12], []byte("WEBP"))
	case "bmp":
		return bytes.HasPrefix(h, []byte("BM"))
	case "avif":
		return len(h) >= 12 && bytes.Equal(h[4:8], []byte("ftyp")) && (bytes.Equal(h[8:12], []byte("avif")) || bytes.Equal(h[8:12], []byte("avis")))
	case "ico":
		return bytes.HasPrefix(h, []byte{0, 0, 1, 0})
	case "svg":
		return isText(h) && bytes.Contains(bytes.ToLower(h), []byte("<svg"))
	case "txt", "md", "csv":
		return isText(h)
	}
	return false
}

func isText(h []byte) bool {
	if bytes.HasPrefix(h, []byte("MZ")) || bytes.HasPrefix(h, []byte("\x7fELF")) {
		return false
	}
	return bytes.IndexByte(h, 0) < 0
}

package main

import "testing"

func TestHeadMatches(t *testing.T) {
	exe := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff")
	cases := []struct {
		ext  string
		head []byte
		want bool
	}{
		{"pdf", []byte("%PDF-1.7\n"), true},
		{"pdf", exe, false}, // renamed .exe
		{"pptx", []byte("PK\x03\x04\x14\x00"), true},
		{"xlsx", []byte("%PDF-1.4"), false},
		{"xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0}, true},
		{"png", []byte("\x89PNG\r\n\x1a\n...."), true},
		{"jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, true},
		{"gif", []byte("GIF89a"), true},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), true},
		{"avif", []byte("\x00\x00\x00\x1cftypavif"), true},
		{"svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), true},
		{"svg", []byte("hello"), false},
		{"txt", []byte("纯文本 note"), true},
		{"txt", exe, false},
		{"md", []byte("# t\x00x"), false},
		{"csv", []byte("a,b\n1,2"), true},
		{"exe", exe, false},
	}
	for _, c := range cases {
		if got := headMatches(c.head, c.ext); got != c.want {
			t.Errorf("headMatches(%q, .%s) = %v, want %v", c.head, c.ext, got, c.want)
		}
	}
}

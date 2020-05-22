package cpio

import (
	"bytes"
	"testing"
)

// l   src      dst   0777  1234  4321
// cl  regular        0640  1234  4321  regular file
// d   dir            0755  1234  4321
var want = [10240]byte{
	0x30, 0x37, 0x30, 0x37, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x61, 0x31, 0x66, 0x66, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x34, 0x64, 0x32, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30,
	0x65, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x33, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x34, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x64, 0x73, 0x74, 0x00, 0x00, 0x00, 0x73, 0x72, 0x63, 0x00,
	0x30, 0x37, 0x30, 0x37, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x32, 0x30, 0x30, 0x30, 0x30, 0x38, 0x31, 0x61, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x34, 0x64, 0x32, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30,
	0x65, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x63, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x38, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x72, 0x65, 0x67, 0x75, 0x6c, 0x61, 0x72, 0x00, 0x00, 0x00,
	0x72, 0x65, 0x67, 0x75, 0x6c, 0x61, 0x72, 0x20, 0x66, 0x69, 0x6c, 0x65,
	0x30, 0x37, 0x30, 0x37, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x33, 0x30, 0x30, 0x30, 0x30, 0x34, 0x31, 0x65, 0x64, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x34, 0x64, 0x32, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30,
	0x65, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x35, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x64, 0x69, 0x72, 0x2f, 0x00, 0x00, 0x30, 0x37, 0x30, 0x37,
	0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x62, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x54, 0x52,
	0x41, 0x49, 0x4c, 0x45, 0x52, 0x21, 0x21, 0x21,
}

func TestWriter(t *testing.T) {
	type entry struct {
		header   *Header
		contents string
	}

	entries := []*entry{
		{
			header: &Header{
				Name: "dst",
				Uid:  1234,
				Gid:  4321,
				Size: 3, // src size
				Mode: 0777,
				Type: TypeSymlink,
			},
			contents: "src",
		},
		{
			header: &Header{
				Name: "regular",
				Uid:  1234,
				Gid:  4321,
				Size: 12,
				Mode: 0640,
				Type: TypeRegular,
			},
			contents: "regular file",
		},
		{
			header: &Header{
				Name: "dir/",
				Uid:  1234,
				Gid:  4321,
				Size: 0,
				Mode: 0755,
				Type: TypeDir,
			},
		},
	}

	b := new(bytes.Buffer)
	w := NewWriter(b)
	for _, v := range entries {
		if err := w.WriteHeader(v.header); err != nil {
			t.Fatal(err)
		}
		if v.contents == "" {
			continue
		}
		if _, err := w.Write([]byte(v.contents)); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()

	if !bytes.Equal(want[:], b.Bytes()) {
		t.Fatal("have != golden")
	}
}

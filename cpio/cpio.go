// original code by github.com/surma/gocpio
// modifications by tlahdekorpi (tuomas.lahdekorpi@gmail.com)
// https://people.freebsd.org/~kientzle/libarchive/man/cpio.5.txt

package cpio

import (
	"errors"
	"io"
)

var (
	errInvalidLength = errors.New("cpio: invalid length")
	errInvalidSize   = errors.New("cpio: invalid size")
	errPartialWrite  = errors.New("cpio: partial write")
	errTooManyBytes  = errors.New("cpio: too many bytes")
)

const (
	TypeFifo    = 001
	TypeChar    = 002
	TypeDir     = 004
	TypeBlock   = 006
	TypeRegular = 010
	TypeSymlink = 012
	TypeSocket  = 014
)

const (
	// cpio newc format magic
	newcMagic = "070701"
	// base16
	digits = "0123456789abcdef"

	zsize = 10240
)

var zeroBlock [zsize]byte

type Header struct {
	Mode     int    // permission and mode bits.
	Uid      int    // user id of owner.
	Gid      int    // group id of owner.
	Mtime    int64  // modified time; seconds since epoch.
	Size     int64  // length in bytes.
	Devmajor int    // major number of character or block device.
	Devminor int    // minor number of character or block device.
	Type     int    // filetype.
	Name     string // name of header file entry.
}

func (hdr *Header) filemode() int {
	return hdr.Mode&0xFFF | (hdr.Type&0xF)<<12
}

type Writer struct {
	w         io.Writer
	inode     int64
	length    int64
	remaining int64
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w, inode: 1}
}

func fmt16(n int64) []byte {
	r := []byte{'0', '0', '0', '0', '0', '0', '0', '0'}
	i := len(r)

	for n >= 16 {
		i--
		r[i] = digits[n&15]
		n >>= 4
	}
	i--
	r[i] = digits[n]

	return r
}

func newcHeader(u ...int64) []byte {
	ret := []byte(newcMagic)
	for _, v := range u {
		ret = append(ret, fmt16(v)...)
	}
	return ret
}

func (cw *Writer) header(hdr *Header) []byte {
	ret := newcHeader(
		cw.inode,
		int64(hdr.filemode()),
		int64(hdr.Uid),
		int64(hdr.Gid),
		0,
		hdr.Mtime,
		hdr.Size,               // filesize
		0,                      // devmajor
		1,                      // devminor
		int64(hdr.Devmajor),    // rdevmajor
		int64(hdr.Devminor),    // rdevminor
		int64(len(hdr.Name))+1, // namesize + zero
		0,                      // check, 0 in newc
	)

	// name + zero
	nb := append([]byte(hdr.Name), byte(0))
	ret = append(ret, nb...)

	return ret
}

func (cw *Writer) flush() error {
	if cw.length == 0 {
		return nil
	}

	if err := cw.zeros(cw.remaining); err != nil {
		return err
	}

	if err := cw.pad(4); err != nil {
		return err
	}

	return nil
}

func (cw *Writer) WriteHeader(hdr *Header) error {
	if hdr.Size < 0 {
		return errInvalidSize
	}

	// flush last file
	if err := cw.flush(); err != nil {
		return err
	}

	// write header bytes
	b := cw.header(hdr)
	n, err := cw.write(b)
	if err != nil {
		return err
	}
	if int(n) != len(b) {
		return errPartialWrite
	}

	// bump inode, set remaining bytes for file
	cw.inode++
	cw.remaining = hdr.Size

	return cw.pad(4)
}

func (cw *Writer) zeros(l int64) error {
	if l > zsize {
		return errInvalidLength
	}

	n, err := cw.write(zeroBlock[:l])
	if err != nil {
		return err
	}
	if n != int(l) {
		return errPartialWrite
	}

	return nil
}

// brings the length of the file to a multiple of mod
func (cw *Writer) pad(mod int64) error {
	return cw.zeros((mod - (cw.length % mod)) % mod)
}

func (cw *Writer) write(b []byte) (int, error) {
	n, err := cw.w.Write(b)
	if err != nil {
		return -1, err
	}

	cw.length += int64(n)
	return n, err
}

func (cw *Writer) Write(b []byte) (int, error) {
	if int64(len(b)) > cw.remaining {
		return -1, errTooManyBytes
	}
	n, err := cw.write(b)
	if err != nil {
		return -1, err
	}

	cw.remaining -= int64(n)
	return n, err
}

var trailer []byte = append(
	newcHeader(0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 11, 0),
	"TRAILER!!!"...,
)

func (cw *Writer) Close() error {
	// flush remaining file
	if err := cw.flush(); err != nil {
		return err
	}

	n, err := cw.write(trailer)
	if err != nil {
		return err
	}
	if int(n) != len(trailer) {
		return errPartialWrite
	}

	return cw.pad(zsize)
}

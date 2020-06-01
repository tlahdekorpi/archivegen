package archive

import (
	"archive/tar"
	"io"
	"os"

	"github.com/tlahdekorpi/archivegen/cpio"
)

type FileType int

const (
	TypeDir FileType = iota
	TypeFifo
	TypeChar
	TypeBlock
	TypeRegular
	TypeSymlink
	TypeSocket
)

type Header struct {
	Name string // name of header file entry.
	Mode int64  // permission and mode bits.
	Uid  int    // user id of owner.
	Gid  int    // group id of owner.
	Size int64  // length in bytes.
	Type FileType
	Time int64
}

type Writer interface {
	io.WriteCloser
	WriteHeader(hdr *Header) error
	Symlink(src, dst string, uid, gid, mode int) error
	WriteFile(file *os.File, hdr *Header) error
}

func NewWriter(format string, w io.Writer) Writer {
	switch format {
	case "tar":
		return &tarWriter{tar.NewWriter(w)}
	case "cpio":
		return &cpioWriter{cpio.NewWriter(w)}
	}
	return nil
}

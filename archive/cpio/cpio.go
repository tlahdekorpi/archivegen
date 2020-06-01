package cpio

import (
	"io"
	"os"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/cpio"
)

type writer struct {
	cw *cpio.Writer
}

func NewWriter(w io.Writer) archive.Writer {
	return &writer{cw: cpio.NewWriter(w)}
}

func (w *writer) Close() error {
	return w.cw.Close()
}

func (w *writer) Write(b []byte) (int, error) {
	return w.cw.Write(b)
}

func (w *writer) WriteFile(file *os.File, hdr *archive.Header) error {
	return w.cw.WriteFile(file, hdrconv(hdr))
}

func typeconv(t archive.FileType) int {
	switch t {
	case archive.TypeDir:
		return cpio.TypeDir
	case archive.TypeFifo:
		return cpio.TypeFifo
	case archive.TypeChar:
		return cpio.TypeChar
	case archive.TypeBlock:
		return cpio.TypeBlock
	case archive.TypeRegular:
		return cpio.TypeRegular
	case archive.TypeSymlink:
		return cpio.TypeSymlink
	case archive.TypeSocket:
		return cpio.TypeSocket
	}
	panic("type")
}

func hdrconv(a *archive.Header) *cpio.Header {
	const max = int64(^uint32(0))

	if a.Size >= max {
		panic("filesize " + a.Name)
	}
	if a.Mode >= max {
		panic("filemode " + a.Name)
	}
	if a.Time >= max {
		panic("mtime " + a.Name)
	}

	return &cpio.Header{
		Name:  a.Name,
		Uid:   a.Uid,
		Gid:   a.Gid,
		Size:  a.Size,
		Mode:  int(a.Mode),
		Type:  typeconv(a.Type),
		Mtime: a.Time,
	}
}

func (w *writer) WriteHeader(hdr *archive.Header) error {
	if hdr.Type == archive.TypeDir {
		hdr.Name += "/"
	}
	return w.cw.WriteHeader(hdrconv(hdr))
}

func (w *writer) Symlink(src, dst string, uid, gid, mode int) error {
	hdr := &cpio.Header{
		Name: dst,
		Size: int64(len(src)),
		Mode: mode,
		Uid:  uid,
		Gid:  gid,
		Type: cpio.TypeSymlink,
	}
	if err := w.cw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := w.Write([]byte(src)); err != nil {
		return err
	}
	return nil
}

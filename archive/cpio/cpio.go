package cpio

import (
	"io"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/cpio"
)

const max32 = int64(^uint32(0))

type writer struct {
	cw *cpio.Writer
	t  bool
}

func NewWriter(w io.Writer, timestamp bool) archive.Writer {
	cw := cpio.NewWriter(w)
	return &writer{
		cw: cw,
		t:  timestamp,
	}
}

func (w *writer) Close() error {
	return w.cw.Close()
}

func (w *writer) Write(b []byte) (int, error) {
	return w.cw.Write(b)
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
	default:
		panic("unknown type " + t.String())
	}

}

func hdrconv(a *archive.Header, t bool) *cpio.Header {
	var mtime int64
	if t && a.Type == archive.TypeRegular {
		mtime = a.ModTime.Unix()
	}

	if a.Size >= max32 {
		panic("filesize " + a.Name)
	}
	if a.Mode >= max32 {
		panic("filemode " + a.Name)
	}
	if mtime >= max32 {
		panic("mtime " + a.Name)
	}

	return &cpio.Header{
		Name:  a.Name,
		Uid:   a.Uid,
		Gid:   a.Gid,
		Size:  a.Size,
		Mode:  int(a.Mode),
		Type:  typeconv(a.Type),
		Mtime: mtime,
	}
}

func (w *writer) WriteHeader(hdr *archive.Header) error {
	return w.cw.WriteHeader(hdrconv(hdr, w.t))
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

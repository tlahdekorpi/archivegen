package archive

import (
	"os"

	"github.com/tlahdekorpi/archivegen/cpio"
)

type cpioWriter struct {
	cw *cpio.Writer
}

func (w *cpioWriter) Close() error {
	return w.cw.Close()
}

func (w *cpioWriter) Write(b []byte) (int, error) {
	return w.cw.Write(b)
}

func (w *cpioWriter) WriteFile(file *os.File, hdr *Header) error {
	return w.cw.WriteFile(file, cpioHeader(hdr))
}

func cpioType(t FileType) int {
	switch t {
	case TypeDir:
		return cpio.TypeDir
	case TypeFifo:
		return cpio.TypeFifo
	case TypeChar:
		return cpio.TypeChar
	case TypeBlock:
		return cpio.TypeBlock
	case TypeRegular:
		return cpio.TypeRegular
	case TypeSymlink:
		return cpio.TypeSymlink
	case TypeSocket:
		return cpio.TypeSocket
	}
	panic("type")
}

func cpioHeader(a *Header) *cpio.Header {
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
		Type:  cpioType(a.Type),
		Mtime: a.Time,
	}
}

func (w *cpioWriter) WriteHeader(hdr *Header) error {
	if hdr.Type == TypeDir {
		hdr.Name += "/"
	}
	return w.cw.WriteHeader(cpioHeader(hdr))
}

func (w *cpioWriter) Symlink(src, dst string, uid, gid, mode int) error {
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

package tar

import (
	"io"
	"os"
	"time"

	"archive/tar"

	"github.com/tlahdekorpi/archivegen/archive"
)

type writer struct {
	tw *tar.Writer
}

func NewWriter(w io.Writer) archive.Writer {
	return &writer{tw: tar.NewWriter(w)}
}

func (w *writer) Close() error {
	return w.tw.Close()
}

func (w *writer) Write(b []byte) (int, error) {
	return w.tw.Write(b)
}

func (w *writer) WriteFile(file *os.File, hdr *archive.Header) error {
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := io.Copy(w.tw, file)
	return err
}

func typeconv(t archive.FileType) byte {
	switch t {
	case archive.TypeDir:
		return tar.TypeDir
	case archive.TypeFifo:
		return tar.TypeFifo
	case archive.TypeChar:
		return tar.TypeChar
	case archive.TypeBlock:
		return tar.TypeBlock
	case archive.TypeRegular:
		return tar.TypeReg
	case archive.TypeSymlink:
		return tar.TypeSymlink
	}
	panic("type")
}

func hdrconv(a *archive.Header) *tar.Header {
	r := &tar.Header{
		Name:     a.Name,
		Uid:      a.Uid,
		Gid:      a.Gid,
		Size:     a.Size,
		Mode:     a.Mode,
		Typeflag: typeconv(a.Type),
	}
	if a.Time > 0 {
		r.ModTime = time.Unix(a.Time, 0)
	}
	return r
}

func (w *writer) WriteHeader(hdr *archive.Header) error {
	if hdr.Type == archive.TypeDir {
		hdr.Name += "/"
	}
	return w.tw.WriteHeader(hdrconv(hdr))
}

func (w *writer) Symlink(src, dst string, uid, gid, mode int) error {
	hdr := &tar.Header{
		Name:     dst,
		Linkname: src,
		Size:     0,
		Mode:     int64(mode),
		Uid:      uid,
		Gid:      gid,
		Typeflag: tar.TypeSymlink,
	}
	if err := w.tw.WriteHeader(hdr); err != nil {
		return err
	}
	return nil
}

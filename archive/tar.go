package archive

import (
	"io"
	"os"
	"time"

	"archive/tar"
)

var Pad bool

type tarWriter struct {
	tw *tar.Writer
}

func (w *tarWriter) Close() error {
	return w.tw.Close()
}

func (w *tarWriter) Write(b []byte) (int, error) {
	return w.tw.Write(b)
}

func (w *tarWriter) writeFile(file *os.File, hdr *Header) error {
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := io.Copy(w.tw, file)
	return err
}

func tarType(t FileType) byte {
	switch t {
	case TypeDir:
		return tar.TypeDir
	case TypeFifo:
		return tar.TypeFifo
	case TypeChar:
		return tar.TypeChar
	case TypeBlock:
		return tar.TypeBlock
	case TypeRegular:
		return tar.TypeReg
	case TypeSymlink:
		return tar.TypeSymlink
	}
	panic("type")
}

func tarHeader(a *Header) *tar.Header {
	r := &tar.Header{
		Name:     a.Name,
		Uid:      a.Uid,
		Gid:      a.Gid,
		Size:     a.Size,
		Mode:     a.Mode,
		Typeflag: tarType(a.Type),
	}
	if a.Time > 0 {
		r.ModTime = time.Unix(a.Time, 0)
	}
	return r
}

func (w *tarWriter) WriteHeader(hdr *Header) error {
	if hdr.Type == TypeDir {
		hdr.Name += "/"
	}
	return w.tw.WriteHeader(tarHeader(hdr))
}

func (w *tarWriter) Symlink(src, dst string, uid, gid, mode int) error {
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

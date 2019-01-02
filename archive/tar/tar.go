package tar

import (
	"io"

	"archive/tar"

	"github.com/tlahdekorpi/archivegen/archive"
)

type writer struct {
	tw *tar.Writer
	t  bool
}

func NewWriter(w io.Writer, timestamp bool) archive.Writer {
	tw := tar.NewWriter(w)
	return &writer{
		tw: tw,
		t:  timestamp,
	}
}

func (w *writer) Close() error {
	return w.tw.Close()
}

func (w *writer) Write(b []byte) (int, error) {
	return w.tw.Write(b)
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
	default:
		panic("unknown type " + t.String())
	}

}

func hdrconv(a *archive.Header, t bool) *tar.Header {
	r := &tar.Header{
		Name:     a.Name,
		Uid:      a.Uid,
		Gid:      a.Gid,
		Size:     a.Size,
		Mode:     a.Mode,
		Typeflag: typeconv(a.Type),
	}
	if t && a.Type == archive.TypeRegular {
		r.ModTime = a.ModTime
		r.ChangeTime = a.ChangeTime
		r.AccessTime = a.AccessTime
	}
	return r
}

func (w *writer) WriteHeader(hdr *archive.Header) error {
	if hdr.Type == archive.TypeDir {
		hdr.Name += "/"
	}
	return w.tw.WriteHeader(hdrconv(hdr, w.t))
}

func (w *writer) Symlink(src, dst string, uid, gid int) error {
	hdr := &tar.Header{
		Name:     dst,
		Linkname: src,
		Size:     0,
		Mode:     0777,
		Uid:      uid,
		Gid:      gid,
		Typeflag: tar.TypeSymlink,
	}
	if err := w.tw.WriteHeader(hdr); err != nil {
		return err
	}
	return nil
}

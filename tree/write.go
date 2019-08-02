package tree

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/config"
)

func writeFile(w archive.Writer, src, dst string, mode, uid, gid int) error {
	l, err := os.Stat(src)
	if err != nil {
		return err
	}

	m := int64(l.Mode().Perm())
	if mode != 0 {
		m = int64(mode)
	}

	t, err := statt(l)
	if err != nil {
		return err
	}

	hdr := &archive.Header{
		Name:       dst,
		Size:       int64(l.Size()),
		Mode:       int64(m),
		Uid:        uid,
		Gid:        gid,
		Type:       archive.TypeRegular,
		ModTime:    l.ModTime(),
		ChangeTime: t.ctime,
		AccessTime: t.atime,
	}
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		return err
	}

	return nil
}

func writeDir(w archive.Writer, dst string, mode, uid, gid int) error {
	hdr := &archive.Header{
		Name: dst,
		Size: 0,
		Mode: int64(mode),
		Uid:  uid,
		Gid:  gid,
		Type: archive.TypeDir,
	}
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}
	return nil
}

func createFile(w archive.Writer, dst string, mode, uid, gid int, data []byte) error {
	hdr := &archive.Header{
		Name: dst,
		Size: int64(len(data)),
		Mode: int64(mode),
		Uid:  uid,
		Gid:  gid,
		Type: archive.TypeRegular,
	}
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

func Write(e config.Entry, w archive.Writer) error {
	switch e.Type {
	case config.TypeRegular:
		return writeFile(w, e.Src, e.Dst, e.Mode, e.User, e.Group)

	case config.TypeDirectory:
		return writeDir(w, e.Src, e.Mode, e.User, e.Group)

	case config.TypeSymlink:
		return w.Symlink(e.Src, e.Dst, e.User, e.Group, e.Mode)

	case config.TypeBase64:
		d := make([]byte, base64.StdEncoding.DecodedLen(len(e.Data)))
		n, err := base64.StdEncoding.Decode(d, e.Data)
		if err != nil {
			return err
		}
		e.Data = d[:n]
		fallthrough
	case config.TypeCreate, config.TypeCreateNoEndl:
		return createFile(w, e.Dst, e.Mode, e.User, e.Group, e.Data)
	}

	return fmt.Errorf("tree: write error: unknown type %q", e)
}

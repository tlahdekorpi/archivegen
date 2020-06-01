package tree

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/config"
)

func writeFile(w archive.Writer, src, dst string, mode, uid, gid int, time int64) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	fs, err := f.Stat()
	if err != nil {
		return err
	}

	return w.WriteFile(f,
		&archive.Header{
			Name: dst,
			Size: int64(fs.Size()),
			Mode: int64(mode),
			Uid:  uid,
			Gid:  gid,
			Type: archive.TypeRegular,
			Time: time,
		},
	)
}

func writeDir(w archive.Writer, dst string, mode, uid, gid int) error {
	return w.WriteHeader(&archive.Header{
		Name: dst,
		Size: 0,
		Mode: int64(mode),
		Uid:  uid,
		Gid:  gid,
		Type: archive.TypeDir,
	})
}

func createFile(w archive.Writer, dst string, mode, uid, gid int, data []byte, time int64) error {
	if err := w.WriteHeader(&archive.Header{
		Name: dst,
		Size: int64(len(data)),
		Mode: int64(mode),
		Uid:  uid,
		Gid:  gid,
		Type: archive.TypeRegular,
		Time: time,
	}); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func Write(e config.Entry, w archive.Writer) error {
	switch e.Type {
	case config.TypeRegular:
		return writeFile(w, e.Src, e.Dst, e.Mode, e.User, e.Group, e.Time)

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
		return createFile(w, e.Dst, e.Mode, e.User, e.Group, e.Data, e.Time)
	}

	return fmt.Errorf("tree: write error: unknown type %q", e)
}

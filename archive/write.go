package archive

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/tlahdekorpi/archivegen/config"
)

var Hardlink bool

type dinode struct {
	dev uint64
	ino uint64
}

var inomap = make(map[dinode]string)

func patheq(a, b string) string {
	p1, f := filepath.Split(a)
	p2, _ := filepath.Split(b)
	if p1 == p2 {
		return f
	}
	return "/" + a
}

func writeFile(w Writer, src, dst string, mode, uid, gid int, time int64) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	fs, err := f.Stat()
	if err != nil {
		return err
	}

	if Hardlink {
		if s, ok := fs.Sys().(*syscall.Stat_t); ok && s.Nlink > 1 {
			if st, ok := inomap[dinode{s.Dev, s.Ino}]; ok {
				log.Printf("hardlink: %s -> %s", dst, patheq(st, dst))
				return w.Symlink(patheq(st, dst), dst, uid, gid, 0777)
			} else {
				inomap[dinode{s.Dev, s.Ino}] = dst
			}
		}
	}

	return w.WriteFile(f,
		&Header{
			Name: dst,
			Size: int64(fs.Size()),
			Mode: int64(mode),
			Uid:  uid,
			Gid:  gid,
			Type: TypeRegular,
			Time: time,
		},
	)
}

func writeDir(w Writer, dst string, mode, uid, gid int) error {
	return w.WriteHeader(&Header{
		Name: dst,
		Size: 0,
		Mode: int64(mode),
		Uid:  uid,
		Gid:  gid,
		Type: TypeDir,
	})
}

func createFile(w Writer, dst string, mode, uid, gid int, data []byte, time int64) error {
	if err := w.WriteHeader(&Header{
		Name: dst,
		Size: int64(len(data)),
		Mode: int64(mode),
		Uid:  uid,
		Gid:  gid,
		Type: TypeRegular,
		Time: time,
	}); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func Write(e config.Entry, w Writer) error {
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

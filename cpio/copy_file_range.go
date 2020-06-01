// +build copy_file_range

package cpio

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func mod(sz, i int) int {
	return (i - (sz % i)) % i
}

func size(name string) int {
	l := 8 * 13
	l += len(name) + 1
	l += len(newcMagic)
	const _pad = 116
	return _pad + l + mod(l, 4)
}

// TODO: bsize
const align = 4096

func (cw *Writer) cpad(hdr *Header) (bool, error) {
	if hdr.Size < align {
		return true, nil
	}
	sz := mod(int(cw.length)+size(hdr.Name), align)
	sz -= sz % 4
	return false, cw.WriteHeader(&Header{
		Name: ".pad",
		Type: TypeDir,
		Size: int64(sz),
	})
}

func (cw *Writer) WriteFile(file *os.File, hdr *Header) error {
	w, ok := cw.w.(*os.File)
	if !ok {
		return cw.writeFile(file, hdr)
	}

	if Pad {
		skip, err := cw.cpad(hdr)
		if err != nil {
			return err
		}
		if skip {
			return cw.writeFile(file, hdr)
		}
	}

	if err := cw.WriteHeader(hdr); err != nil {
		return err
	}

	if Pad {
		off, err := w.Seek(0, os.SEEK_CUR)
		if err != nil {
			return err
		}
		if n := off % align; n != 0 {
			return fmt.Errorf("not aligned: %d, %s", n, hdr.Name)
		}
	}

	var sz int = int(hdr.Size)
	for {
		n, err := unix.CopyFileRange(int(file.Fd()), nil, int(w.Fd()), nil, sz, 0)
		if err != nil {
			return err
		}
		cw.length += int64(n)
		cw.remaining -= int64(n)
		sz -= n
		if sz <= 0 {
			break
		}
	}
	return nil
}

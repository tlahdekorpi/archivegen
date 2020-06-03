// +build copy_file_range

package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

type fileState interface {
	LogicalRemaining() int64
	PhysicalRemaining() int64
}

type fileWriter interface {
	io.Writer
	fileState

	ReadFrom(io.Reader) (int64, error)
}

type tw struct {
	w    io.Writer
	pad  int64
	curr fileWriter
	hdr  tar.Header
}

type fakeWriter struct{}

func (fakeWriter) LogicalRemaining() int64           { return 0 }
func (fakeWriter) PhysicalRemaining() int64          { return 0 }
func (fakeWriter) Write([]byte) (int, error)         { return -1, errors.New("write") }
func (fakeWriter) ReadFrom(io.Reader) (int64, error) { return -1, errors.New("readfrom") }

var _ fileWriter = fakeWriter{}

// TODO: bsize
const align = 4096

func mod(sz, i int64) int64 {
	return (i - (sz % i)) % i
}

var zb [align]byte

func (w *tarWriter) cpad(hdr *Header, f *os.File, tw *tw) (bool, error) {
	if hdr.Size < align {
		return true, nil
	}

	offset, err := f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return false, err
	}

	const (
		ustarmax = 1<<33 - 1
		ustar    = 512
		pax      = 1024
	)

	var pad int64
	if hdr.Size > ustarmax {
		pad = pax
	} else {
		pad = ustar
	}

	sz := mod(offset+pad+ustar, align)
	sz -= sz % pad
	if err := w.WriteHeader(&Header{
		Name: ".pad",
		Type: TypeRegular,
		Size: sz,
	}); err != nil {
		return false, err
	}

	tw.curr = fakeWriter{}
	_, err = f.Write(zb[:sz])
	return false, err
}

func (w *tarWriter) WriteFile(file *os.File, hdr *Header) error {
	tw := (*tw)(unsafe.Pointer(w.tw))
	f, ok := tw.w.(*os.File)
	if !ok {
		return w.writeFile(file, hdr)
	}

	if Pad {
		if skip, err := w.cpad(hdr, f, tw); err != nil {
			return err
		} else if skip {
			return w.writeFile(file, hdr)
		}
	}

	if err := w.WriteHeader(hdr); err != nil {
		return err
	}

	if Pad {
		off, err := f.Seek(0, os.SEEK_CUR)
		if err != nil {
			return err
		}
		if n := off % align; n != 0 {
			return fmt.Errorf("not aligned: %d, %s", n, hdr.Name)
		}
	}

	tw.curr = fakeWriter{}
	var sz int = int(hdr.Size)
	for {
		n, err := unix.CopyFileRange(int(file.Fd()), nil, int(f.Fd()), nil, sz, 0)
		if err != nil {
			return err
		}
		sz -= n
		if sz <= 0 {
			break
		}
	}
	return nil
}

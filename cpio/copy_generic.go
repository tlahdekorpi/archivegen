// +build !copy_file_range

package cpio

import (
	"io"
	"os"
)

func (cw *Writer) WriteFile(file *os.File, hdr *Header) error {
	if err := cw.WriteHeader(hdr); err != nil {
		return err
	}
	n, err := io.Copy(cw.w, file)
	cw.length += int64(n)
	cw.remaining -= int64(n)
	return err
}

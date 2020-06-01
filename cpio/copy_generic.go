// +build !copy_file_range

package cpio

import "os"

func (cw *Writer) WriteFile(file *os.File, hdr *Header) error {
	return cw.writeFile(file, hdr)
}

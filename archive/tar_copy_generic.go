// +build !copy_file_range

package archive

import "os"

func (w *tarWriter) WriteFile(file *os.File, hdr *Header) error {
	return w.writeFile(file, hdr)
}

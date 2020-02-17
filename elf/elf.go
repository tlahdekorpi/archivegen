package elf

import (
	"debug/elf"
	"errors"
)

var (
	errNoInterp    = errors.New("elf: no interpreter found")
	errPartialRead = errors.New("elf: partial read")
)

type elfFile interface {
	DynString(tag elf.DynTag) ([]string, error)
	Close() error
}

func eopen(file string) (elfFile, error) {
	return elf.Open(file)
}

// overridden in testing
var open = eopen

func readinterp(file elfFile) (string, error) {
	var (
		f  *elf.File
		ok bool
	)

	if f, ok = file.(*elf.File); !ok {
		return "", errNoInterp
	}

	for _, v := range f.Progs {
		if v.Type != elf.PT_INTERP {
			continue
		}

		b := make([]byte, v.Memsz-1)
		n, err := v.ReadAt(b, 0)
		if err != nil {
			return "", err
		}
		if uint64(n) != v.Memsz-1 {
			return "", errPartialRead
		}

		return string(b), nil
	}

	return "", errNoInterp
}

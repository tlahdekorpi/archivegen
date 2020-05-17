package elf

import (
	"debug/elf"
	"errors"
	"sync"
)

var (
	errNoInterp    = errors.New("elf: no interpreter found")
	errPartialRead = errors.New("elf: partial read")
)

type fsfile struct {
	name   string
	interp string
	class  elf.Class
	elf    *elf.File
	file   File
}

func (f *fsfile) Dynamic() (File, error) {
	var err error
	if f.elf == nil {
		return f.file, nil
	}

	if f.file.Runpath, err = f.elf.DynString(elf.DT_RUNPATH); err != nil {
		return f.file, err
	}
	if f.file.Rpath, err = f.elf.DynString(elf.DT_RPATH); err != nil {
		return f.file, err
	}
	if f.file.Needed, err = f.elf.DynString(elf.DT_NEEDED); err != nil {
		return f.file, err
	}

	if err := f.elf.Close(); err != nil {
		return f.file, err
	}
	f.elf = nil

	mu.Lock()
	cache[f.name] = f
	mu.Unlock()
	return f.file, nil
}

func (f *fsfile) Interpreter() (string, error) {
	if f.elf == nil {
		return f.interp, nil
	}

	for _, v := range f.elf.Progs {
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

		f.interp = string(b)
		return f.interp, nil
	}
	return "", errNoInterp
}

func (f *fsfile) Class() elf.Class {
	return f.class
}

func (f *fsfile) Close() error {
	if f.elf == nil {
		return nil
	}
	return f.elf.Close()
}

var mu sync.Mutex
var cache = make(map[string]*fsfile)

func defaultLoader(file, prefix string) (ELF, error) {
	mu.Lock()
	defer mu.Unlock()
	if f, exists := cache[file]; exists {
		return f, nil
	}

	r := &fsfile{
		name: file,
	}

	var err error
	if prefix != "" {
		if file, err = expand(file, prefix); err != nil {
			return nil, err
		}
	}

	r.elf, err = elf.Open(file)
	if err != nil {
		return nil, err
	}

	r.class = r.elf.Class
	return r, nil
}

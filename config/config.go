package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
)

var (
	errInvalidEntry = errors.New("config: invalid entry")
	errIgnoredEntry = errors.New("config: ignored entry")
	errNoArguments  = errors.New("config: no arguments")
)

func split(r rune) bool {
	return r == ' ' || r == '\t'
}

func fieldsFuncN(s string, n int, f func(rune) bool) []string {
	var (
		c   int
		inN bool
		inB bool
	)

	// Count fields to avoid allocations.
	for _, v := range s {
		if n > 0 && c >= n {
			break
		}

		inB = !inN
		inN = !f(v)
		if inN && inB {
			c++
		}
	}

	// Create slice with the expected length.
	ret := make([]string, c)

	var (
		na int
		fs int
		is bool
	)

	// Set to -1 when looking for start of field.
	fs = -1

	for k, v := range s {
		// After Nth field, all remaining is the last field.
		if n > 0 && na >= c-1 {
			fs = k
			break
		}

		is = f(v)

		// In and not at start
		if is && fs > -1 {
			ret[na] = s[fs:k]
			na++
			fs = -1
		}

		// Not in and looking for start.
		if !is && fs < 0 {
			fs = k
		}
	}

	// Add last field.
	if fs > -1 {
		ret[na] = s[fs:]
	}

	return ret
}

func isheredoc(data []string) string {
	const idx = idxData - 1

	if len(data) < idxData {
		return ""
	}

	if len(data[idx]) <= 2 {
		return ""
	}

	x := strings.TrimLeft(data[idx], " \t")
	if x[:2] != "<<" {
		return ""
	}

	return x[2:]
}

var multiReplace = strings.NewReplacer(
	"\t", "",
	"\n", ",",
).Replace

func multireader(s *bufio.Scanner, entry []string, eof string, t byte) ([]string, int, error) {
	var (
		idx int
		ret string
	)

	switch t {
	case 'c':
		idx = idxData - 1
	case 'l':
		idx = idxDst
		if len(entry) <= idxDst {
			idx = idxSrc
		}
		ret = entry[idx]
	default:
		idx = idxSrc
		ret = entry[idx]
	}

	var (
		n int
		r string
	)

	for s.Scan() {
		n++

		if err := s.Err(); err != nil {
			return entry, n, err
		}

		r = s.Text()

		switch t {
		case 'c':
			if r != eof {
				ret += r + "\n"
				continue
			}
		default:
			r = strings.TrimSpace(r)
			if len(r) == 0 {
				continue
			}
			if r[0] == '#' {
				continue
			}
			if r[0] != '}' {
				ret += r + "\n"
				continue
			}
		}

		break
	}

	ret = strings.TrimSuffix(ret, "\n")

	if t != 'c' {
		rr := fieldsFuncN(r, -1, split)
		ret += rr[0]
		ret = multiReplace(ret)
		entry = append(entry, rr[1:]...)
	} else {
		entry = append(entry, eof)
	}

	entry[idx] = ret
	return entry, n, nil
}

// TODO: error handling
func fromReader(rootfs *string, vars []string, r io.Reader) *Map {
	s := bufio.NewScanner(r)
	m := newMap(vars)

	var n int
	for s.Scan() {
		n++
		if err := s.Err(); err != nil {
			log.Printf("error: %q, line %d", err, n)
			return nil
		}

		d := s.Text()
		if len(d) < 1 {
			continue
		}
		if d[0] == '\n' || d[0] == '#' {
			continue
		}

		var (
			i   int
			err error
			f   []string
		)

		switch d[0] {
		case 'c':
			f = fieldsFuncN(d, idxData, split)
			if eof := isheredoc(f); eof != "" {
				f, i, err = multireader(s, f, eof, d[0])
			}
		default:
			f = fieldsFuncN(d, -1, split)
			if d[len(d)-1] == '{' {
				f, i, err = multireader(s, f, "}", d[0])
			}
		}

		n += i

		if err != nil {
			log.Printf("error: %s, line %d", err, n)
			return nil
		}

		if len(f) < 2 && f[idxType] != maskClear {
			log.Printf("error: %s, line %d", errNoArguments, n)
			continue
		}

		if err := m.add(f, rootfs); err != nil {
			log.Printf("error: %s, line %d", err, n)
			return nil
		}
	}

	return m
}

func FromReader(vars []string, r io.Reader) *Map {
	return fromReader(nil, vars, r)
}

func FromReaderRoot(rootfs string, vars []string, r io.Reader) *Map {
	if rootfs != "" {
		return fromReader(&rootfs, vars, r)
	}
	return fromReader(nil, vars, r)
}

func fromFiles(rootfs *string, vars []string, files ...string) (*Map, error) {
	cfg := newMap(vars)
	for _, v := range files {
		f, err := os.Open(path.Clean(v))
		if err != nil {
			return nil, err
		}

		// TODO: err
		m := FromReaderRoot(*rootfs, vars, f)

		if m == nil {
			return nil, fmt.Errorf("error")
		}

		if err := cfg.Merge(m); err != nil {
			return nil, err
		}

		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func FromFiles(vars []string, files ...string) (*Map, error) {
	return fromFiles(nil, vars, files...)
}

func FromFilesRoot(rootfs string, vars []string, files ...string) (*Map, error) {
	return fromFiles(&rootfs, vars, files...)
}

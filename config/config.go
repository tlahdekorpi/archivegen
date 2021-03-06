package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tlahdekorpi/archivegen/elf"
)

var (
	errInvalidEntry = errors.New("invalid entry")
	errIgnoredEntry = errors.New("ignored entry")
	errNoArguments  = errors.New("no arguments")
)

type split rune

func (s *split) split(r rune) bool {
	ret := rune(*s) != '\\' && r == ' ' || r == '\t'
	*s = split(r)
	return ret
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
		rr := fieldsFuncN(r, -1, new(split).split)
		ret += rr[0]
		ret = multiReplace(ret)
		entry = append(entry, rr[1:]...)
	} else {
		entry = append(entry, eof)
	}

	entry[idx] = ret
	return entry, n, nil
}

type lineError struct {
	line int
	err  error
}

func (l lineError) Error() string {
	return fmt.Sprintf("%s, line %d", l.err.Error(), l.line)
}

func failable(e []string) (ok bool) {
	if ok = e[idxType][0] == '?'; ok {
		e[idxType] = e[idxType][1:]
	}
	return
}

type Config struct {
	Resolver *elf.Resolver
	Prefix   string
	Vars     []string
}

func (c *Config) FromReader(r io.Reader) (*Map, error) {
	s := bufio.NewScanner(r)
	m := c.newMap()

	var n int
	for s.Scan() {
		n++
		if err := s.Err(); err != nil {
			return nil, lineError{n, err}
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
			f = fieldsFuncN(d, idxData, new(split).split)
			if eof := isheredoc(f); eof != "" {
				f, i, err = multireader(s, f, eof, d[0])
			}
			if x := len(f); x < idxData {
				f[x-1] = strings.TrimSpace(f[x-1])
			}
		default:
			f = fieldsFuncN(d, -1, new(split).split)
			if d[len(d)-1] == '{' {
				f, i, err = multireader(s, f, "}", d[0])
			}
		}

		n += i

		if err != nil {
			return nil, lineError{n, err}
		}

		if len(f) < 2 && f[idxType] != maskClear {
			return nil, lineError{n, errNoArguments}
		}

		fail := failable(f)
		if err := m.add(f, fail, n); err != nil {
			return nil, lineError{n, err}
		}
	}

	if err := s.Err(); err != nil {
		return nil, lineError{n, err}
	}

	if Opt.ELF.Concurrent {
		if err := m.includeElfs(); err != nil {
			return nil, err
		}
	}

	return m, nil
}

func (c *Config) FromFiles(files ...string) (*Map, error) {
	cfg := c.newMap()
	for k, v := range files {
		var (
			f   io.ReadCloser
			err error
		)
		if v == "-" {
			f = os.Stdin
		} else {
			f, err = os.Open(v)
		}
		if err != nil {
			return nil, err
		}

		m, err := c.FromReader(f)
		if err != nil {
			return nil, fmt.Errorf("%s (%d): %v", v, k, err)
		}

		if err := cfg.Merge(m); err != nil {
			return nil, fmt.Errorf("%s (%d): %v", v, k, err)
		}

		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("%s (%d): %v", v, k, err)
		}
	}

	return cfg, nil
}

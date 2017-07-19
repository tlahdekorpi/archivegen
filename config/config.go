package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
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

// TODO: error handling
func FromReader(r io.Reader) *Map {
	s := bufio.NewScanner(r)
	m := newMap()

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

		var f []string
		if d[0] != 'c' {
			f = fieldsFuncN(d, -1, split)
		} else {
			f = fieldsFuncN(d, idxData, split)
		}

		if len(f) < 2 {
			log.Printf("error: %s, line %d", errNoArguments, n)
			continue
		}
		if err := m.add(f); err != nil {
			log.Printf("error: %s, line %d", err, n)
			return nil
		}
	}

	return m
}

func FromFiles(files ...string) (*Map, error) {
	cfg := newMap()
	for _, v := range files {
		f, err := os.Open(path.Clean(v))
		if err != nil {
			return nil, err
		}

		c := FromReader(f)
		if c == nil {
			return nil, fmt.Errorf("error")
		}

		if err := cfg.Merge(c); err != nil {
			return nil, err
		}

		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

`archivegen` is a tool to generate tar or cpio archives using a tmpfiles-like syntax.

The tool can be used to make initramfs or container images with tools like [buildah](https://github.com/containers/buildah).

## Usage
Configuration can be read from stdin or from commandline arguments. `-print` can be used to inspect the resolved archive including ELF dependencies, recursive entries, etc.

Archives created from `-print` output results in the same archive.

## Configuration file format
The configuration format is a simple line per entry with arguments separated by whitespace. [examples](https://github.com/tlahdekorpi/archivegen/tree/master/examples)

### Types
Types suffixed with `r` are relative to `-rootfs` except `L`. `LA` can be used for absolute paths with ELFs, runpath/rpath $ORIGIN is also absolute when using `LA`. Arguments can be omitted with `-`.

`*` required

**`$`** Variable, can be defined or overridden with `-X` from commandline
```sh
# $ *name value
$ a foo
# variables can be used to define variables
$ b bar
$ c $a$b
d $c
```

**`d`** Directory
```sh
# d *dst mode uid gid
# all non-existent diretories below are created with the same permissions
d a/b/c 0700 1 1
```

**`l`** Symlink
```sh
# l *src *dst mode uid gid
# source and destination are reversed
# usr/bin/sh is a symlink to busybox
l busybox usr/bin/sh
```

**`f, fr`** File
```sh
# f *src dst mode uid gid
f /a/b/c/file foo/bar/baz

# destination is the source when omitted
f /a/b/c/file

# files prefixed with ? are omitted if they don't exist
?fr /optional
```

**`R, Rr`** Recursive
```sh
# R *src dst uid gid
# destination is the source when not specified
R a/b/c

# when destination is omitted, source is stripped from destination
R dir -

# source path is stripped from destination when specified
R a/b/c foo
# a/b/c/file -> foo/file
```

**`r, rr`** Regex
```sh
# r *src dst uid gid
# all matches are prefixed with dst
r /a/.*/c dst
# /a/b/c -> dst/a/b/c
r /a/.*/c -
# /a/b/c -> a/b/c
```

**`c, cl, b64`** Create
```sh
# c *dst mode uid gid data
c file - - - contents
# trailing newline is omitted from argument
cl file - - - contents
# binary data can added as base64
b64 file - - - Y29udGVudHMK
# argument can be a heredoc, variables are expanded inside heredoc
$ variable data
c file - - - <<!
$variable
!
```

**`L, LA, rL, i`** ELF
```sh
# L *src dst mode uid gid
# src and all dependencies are read from rootfs
# -rootfs /foo -> /foo/usr/bin/bash
L /usr/bin/bash
# using LA, src and dependencies from rpath/runpath containing $ORIGIN are not prefixed with rootfs

# rL *src - - uid gid
# dst and mode are ignored
rL /usr/lib/httpd/modules/.so$

# search library paths
i libnss_files.so.2
```

### Repeating entries
Entry source can be repeated using braces (nesting not supported), only symlink destination can be repeated.

```sh
# destination is the source when not specified
f file{1,2}

# destination is the source file joined to the argument
f a/b/c/file{1,2} foo
# foo/file1
# foo/file2

# variables can be used in repeating arguments
$ bin sh,cat,ls
l busybox usr/bin/{$bin}

# arguments can span multiple lines
l ../{
	foo
	bar
} baz
# baz/foo -> ../foo
```

### Masks
Masks can be used to rewrite, modify or exclude entries from archives using regular expressions. https://golang.org/s/re2syntax

**`mm`** Mode
```sh
# mm *idx *regexp mode uid gid
# mask all files to be owned by root
mm - . - 0 0
# index can be omitted or used to override current masks
mm 0 . - 100 100
```

**`mr`** Rewrite
```sh
# mr *idx *regexp *dst
mr - a/b/c foo
d a/b/c/bar
# -> foo/bar
```

**`mi`, `mI`** Ignore
```sh
# mi *idx *regexp
# a
# ├── b
# │   ├── file1
# │   └── file2
# └── file3

# ignore b
mi - foo/b
# ignore everything except b 
mI - foo/b
# recursively add a to foo
R a foo
```

**`mc`** Clear
```sh
# mc idx
# clear all masks
mc
# clear the last mask
mc -
# clear the last n masks
mc -2
```

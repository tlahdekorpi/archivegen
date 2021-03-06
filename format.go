package main

const helpFormat = `
Base64     b64  *dst  mode uid  gid data
Create     c,cl *dst  mode uid  gid data
ELF        L,rL *src  dst  -    uid gid
Path       p    *src  dst  -    uid gid
Library    i    *src  -    -    uid gid
File       f,fr *src  dst  mode uid gid
Auto       a,ar *src  dst  mode uid gid
Symlink    l    *dst *src  mode uid gid
Regex      r,rr *src *dst  uid  gid
Recursive  R,Rr *src *dst  uid  gid
Directory  d    *dst  mode uid  gid

Mode    mm    *idx *regexp  mode uid gid
Rename  mr    *idx *regexp *dst
Ignore  mi,mI *idx *regexp
Clear   mc     idx`

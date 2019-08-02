package main

const helpFormat = `
Base64     b64  *dst  mode uid  gid data
Create     c,cl *dst  mode uid  gid data
ELF        L,gL *src  dst  mode uid gid
File       f,fr *src  dst  mode uid gid
Symlink    l    *dst *src  mode uid gid
Glob       g,gr *src *dst  uid  gid
Recursive  R,Rr *src *dst  uid  gid
Directory  d    *dst  mode uid  gid

Mode    mm    *idx *regexp  mode uid gid
Rename  mr    *idx *regexp *dst
Ignore  mi,mI *idx *regexp
Clear   mc     idx`

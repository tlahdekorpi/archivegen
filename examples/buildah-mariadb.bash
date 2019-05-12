#!/bin/bash
set -ex

name=mariadb
uid=1000
config=(
        --user $uid
        --port 3306
        --entrypoint '["/usr/bin/run"]'
)

fimg=$(buildah from fedora)
fdir=$(buildah mount $fimg)
buildah run $fimg -- dnf -y install busybox mariadb{,-server}

img=$(buildah from scratch)
dir=$(buildah mount $img)
archivegen -rootfs "$fdir" -X "uid=$uid" -stdout mariadb.archive | bsdtar xf - -C $dir

id=$(buildah umount $img)
buildah config "${config[@]}" $id
buildah commit --rm $id $name
buildah umount $fimg

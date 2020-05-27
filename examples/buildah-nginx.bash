#!/bin/bash
set -ex

name=nginx
config=(
        --user 1000
        --port 8000
        --entrypoint '["/usr/bin/nginx"]'
)

img=$(buildah from scratch)
dir=$(buildah mount $img)

archivegen <<!archive | bsdtar xf - -C $dir
p nginx usr/bin/
c data/html/index.html - - - hello world
c etc/nginx/nginx.conf - - - <<!
daemon off;
events {}
http {
        server {
                listen 8000;
                location / {
                        root  /data/html;
                        index index.html;
                }
        }
}
pid /dev/shm/nginx.pid;
!

l /dev/stdout var/log/nginx/{error,access}.log
l /dev/shm    var/lib/nginx/{fastcgi,proxy,client-body,uwsgi,scgi,tmp}
!archive

id=$(buildah umount $img)
buildah config "${config[@]}" $id
buildah commit $id $name

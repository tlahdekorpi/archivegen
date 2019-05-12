FROM fedora AS build

COPY archivegen mariadb.archive /

RUN dnf -y install busybox mariadb mariadb-server
RUN mkdir /out && /archivegen -stdout /mariadb.archive | tar xf - -C /out

FROM scratch
COPY --from=build /out /
ENTRYPOINT ["/usr/bin/entrypoint"]

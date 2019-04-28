FROM ubuntu AS build

COPY archivegen mariadb.archive /

RUN apt-get update && apt-get install -y mariadb-server busybox-static
RUN mkdir /out && /archivegen -stdout /mariadb.archive | tar xf - -C /out

FROM scratch
COPY --from=build /out /
ENTRYPOINT ["/usr/bin/entrypoint"]

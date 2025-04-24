FROM debian:bullseye

# Установим компиляторы и Go
RUN apt update && apt install -y \
  build-essential gcc-mingw-w64 g++-mingw-w64 \
  golang zip unzip make git

ENV GOROOT=/usr/lib/go
ENV GOPATH=/root/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:/usr/local/bin:/usr/bin:/bin

WORKDIR /build
COPY . .

# Исправим go.mod, если где-то встречается go 1.23.0
RUN find . -name go.mod -exec sed -i 's/go 1\.23\.0/go 1.23/' {} \;

RUN chmod +x ./build_static.sh && ./build_static.sh

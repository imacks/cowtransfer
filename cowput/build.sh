#!/bin/sh
set -e

mkdir -p bin

build_targets="windows/amd64 linux/amd64 darwin/amd64"
build_filename="cowput"

for build_target in $build_targets ; do
    build_os=$(echo "$build_target" | cut -d'/' -f1)
    build_arch=$(echo "$build_target" | cut -d'/' -f2)

    build_outfile="./bin/${build_filename}-${build_os}-${build_arch}"
    if [ "$build_target" = "windows/amd64" ]; then
        build_outfile="./bin/${build_filename}.exe"
    elif [ "$build_os" = "darwin" ]; then
        build_outfile="./bin/${build_filename}-osx-${build_arch}"
    fi
    echo "building ${build_outfile}"
    GOOS=$build_os GOARCH=$build_arch go build -ldflags "$build_ldflags" -o "$build_outfile"
done
echo "done!"
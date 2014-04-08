#!/usr/bin/make -f

SHELL=/bin/bash

build: deps
	go build

release: deps golang-crosscompile
	source golang-crosscompile/crosscompile.bash; \
	go-darwin-386 build -o release/zedrem-Darwin-i386; \
	go-darwin-amd64 build -o release/zedrem-Darwin-x86_64; \
	go-linux-386 build -o release/zedrem-Linux-i386; \
	go-linux-386 build -o release/zedrem-Linux-i686; \
	go-linux-amd64 build -o release/zedrem-Linux-x86_64; \
	go-linux-arm build -o release/zedrem-Linux-armv6l; \
	go-freebsd-386 build -o release/zedrem-FreeBSD-i386; \
	go-freebsd-amd64 build -o release/zedrem-FreeBSD-amd64; \
	go-windows-386 build -o release/zedrem.exe

golang-crosscompile:
	git clone https://github.com/davecheney/golang-crosscompile.git

deps:
	go get code.google.com/p/go.net/websocket
	go get code.google.com/p/go-uuid/uuid
	go get code.google.com/p/gcfg

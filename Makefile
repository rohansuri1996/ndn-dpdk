export CGO_CFLAGS_ALLOW := '.*'
ifeq ($(origin CC),default)
	CC = gcc
endif
export CC

.PHONY: all
all: gopkg cmds npm

.PHONY: gopkg
gopkg: godeps
	go build -v ./...

.PHONY: godeps
godeps: build/libndn-dpdk-c.a build/cgodeps.done build/bpf.done

csrc/meson.build mk/meson.build:
	mk/update-list.sh

build/build.ninja: csrc/meson.build mk/meson.build
	bash -c 'source mk/cflags.sh; meson build $$MESONFLAGS'

csrc/dpdk/thread-enum.h: dpdk/ealthread/ctrl.go
	go generate ./$(<D)

csrc/fileserver/enum.h csrc/fileserver/an.h: app/fileserver/config.go ndn/rdr/ndn6file/*.go
	go generate ./$(<D)

csrc/fib/enum.h: container/fib/fibdef/enum.go
	go generate ./$(<D)

csrc/ndni/enum.h csrc/ndni/an.h: ndni/enum.go ndn/an/*.go
	go generate ./$(<D)

csrc/iface/enum.h: iface/enum.go
	go generate ./$(<D)

csrc/pcct/cs-enum.h: container/cs/enum.go
	go generate ./$(<D)

csrc/pdump/enum.h: app/pdump/enum.go
	go generate ./$(<D)

csrc/tgconsumer/enum.h: app/tgconsumer/config.go
	go generate ./$(<D)

csrc/tgproducer/enum.h: app/tgproducer/config.go
	go generate ./$(<D)

.PHONY: build/libndn-dpdk-c.a
build/libndn-dpdk-c.a: build/build.ninja csrc/dpdk/thread-enum.h csrc/fib/enum.h csrc/fileserver/an.h csrc/fileserver/enum.h csrc/ndni/an.h csrc/ndni/enum.h csrc/iface/enum.h csrc/pcct/cs-enum.h csrc/pdump/enum.h csrc/tgconsumer/enum.h csrc/tgproducer/enum.h
	ninja -C build

build/cgodeps.done: build/build.ninja
	ninja -C build cgoflags cgostruct cgotest schema
	touch $@

build/bpf.done: build/build.ninja bpf/**/*.c csrc/strategyapi/* csrc/fib/enum.h
	ninja -C build bpf
	touch $@

.PHONY: cmds
cmds: build/bin/ndndpdk-ctrl build/bin/ndndpdk-godemo build/bin/ndndpdk-hrlog2histogram build/bin/ndndpdk-jrproxy build/bin/ndndpdk-svc

build/bin/%: cmd/%/* godeps
	GOBIN=$$(realpath build/bin) go install "-ldflags=$$(mk/version/ldflags.sh)" ./cmd/$*

.PHONY: npm
npm: build/share/ndn-dpdk/ndn-dpdk.npm.tgz

build/share/ndn-dpdk/ndn-dpdk.npm.tgz:
	node_modules/.bin/tsc
	jq -n '{ type: "module" }' >build/js/package.json
	mv $$(npm pack -s .) $@

.PHONY: install
install:
	mk/install.sh

.PHONY: uninstall
uninstall:
	mk/uninstall.sh

.PHONY: doxygen
doxygen:
	doxygen docs/Doxyfile 2>&1 | docs/filter-Doxygen-warning.awk 1>&2

.PHONY: lint
lint:
	mk/format-code.sh

.PHONY: test
test: godeps
	mk/gotest.sh

.PHONY: coverage
coverage:
	ninja -C build coverage-html

.PHONY: clean
clean:
	awk '!(/node_modules/ || /package-lock/ || /\*/)' .dockerignore | xargs rm -rf
	awk '/\*/' .dockerignore | xargs -I{} -n1 find -wholename ./{} -delete
	go clean -cache ./...

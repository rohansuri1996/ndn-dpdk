#!/bin/bash
set -eo pipefail

NEEDED_BINARIES=(
  curl
  lsb_release
)
MISSING_BINARIES=()

SUDO=
SUDOPKG=
APTSUGGEST='apt install --no-install-recommends'
if [[ -z $SKIPROOTCHECK ]]; then
  NEEDED_BINARIES+=(sudo)
  SUDO=sudo
  SUDOPKG=sudo
  APTSUGGEST='sudo '$APTSUGGEST
  if [[ $(id -u) -eq 0 ]] && [[ -z $SKIPROOTCHECK ]]; then
    echo 'Do not run this script as root'
    echo 'To skip this check, set SKIPROOTCHECK=1 environ'
    exit 1
  fi
fi

for B in "${NEEDED_BINARIES[@]}"; do
  if ! which $B &>/dev/null ; then
    MISSING_BINARIES+=($B)
  fi
done
if [[ ${#MISSING_BINARIES[@]} -gt 0 ]] ; then
  echo "Missing commands (${MISSING_BINARIES[@]}) to start this script. To install:"
  echo "  ${APTSUGGEST} ca-certificates curl lsb-release ${SUDOPKG}"
  exit 1
fi

DFLT_CODEROOT=$HOME/code
DFLT_NODEVER=16.x
DFLT_GOVER=latest
DFLT_UBPFVER=HEAD
DFLT_LIBBPFVER=v0.4.0
DFLT_DPDKVER=21.08
DFLT_KMODSVER=HEAD
DFLT_SPDKVER=21.07
DFLT_URINGVER=liburing-2.0
DFLT_NJOBS=$(nproc)
DFLT_TARGETARCH=native

KERNELVER=$(uname -r)
HAS_KERNEL_HEADERS=0
if [[ -d /usr/src/linux-headers-${KERNELVER} ]]; then
  HAS_KERNEL_HEADERS=1
else
  DFLT_KMODSVER=0
fi

CODEROOT=$DFLT_CODEROOT
NODEVER=$DFLT_NODEVER
GOVER=$DFLT_GOVER
UBPFVER=$DFLT_UBPFVER
LIBBPFVER=$DFLT_LIBBPFVER
DPDKVER=$DFLT_DPDKVER
KMODSVER=$DFLT_KMODSVER
SPDKVER=$DFLT_SPDKVER
URINGVER=$DFLT_URINGVER
NJOBS=$DFLT_NJOBS
TARGETARCH=$DFLT_TARGETARCH

ARGS=$(getopt -o 'hy' --long 'dir:,node:,go:,ubpf:,libbpf:,dpdk:,kmods:,spdk:,uring:,jobs:,arch:' -- "$@")
eval "set -- $ARGS"
while true; do
  case $1 in
    (-h) HELP=1; shift;;
    (-y) CONFIRM=1; shift;;
    (--dir) CODEROOT=$2; shift 2;;
    (--node) NODEVER=$2; shift 2;;
    (--go) GOVER=$2; shift 2;;
    (--ubpf) UBPFVER=$2; shift 2;;
    (--libbpf) LIBBPFVER=$2; shift 2;;
    (--dpdk) DPDKVER=$2; shift 2;;
    (--kmods) KMODSVER=$2; shift 2;;
    (--spdk) SPDKVER=$2; shift 2;;
    (--uring) URINGVER=$2; shift 2;;
    (--jobs) NJOBS=$2; shift 2;;
    (--arch) TARGETARCH=$2; shift 2;;
    (--) shift; break;;
    (*) exit 1;;
  esac
done

if [[ $HELP -eq 1 ]]; then
  cat <<EOT
ndndpdk-depends.sh ...ARGS
  -h  Display help and exit.
  -y  Skip confirmation.
  --dir=${DFLT_CODEROOT}
      Set where to download and compile code.
  --node=${DFLT_NODEVER}
      Set Node.js version. '0' to skip.
  --go=${DFLT_GOVER}
      Set Go version. 'latest' for latest 1.x version. '0' to skip.
  --ubpf=${DFLT_UBPFVER}
      Set uBPF branch or commit SHA. '0' to skip.
  --libbpf=${DFLT_LIBBPFVER}
      Set libbpf branch or commit SHA. '0' to skip.
  --dpdk=${DFLT_DPDKVER}
      Set DPDK version. '0' to skip.
  --kmods=${DFLT_KMODSVER}
      Set DPDK kernel modules branch or commit SHA. '0' to skip.
  --spdk=${DFLT_SPDKVER}
      Set SPDK version. '0' to skip.
  --uring=${DFLT_URINGVER}
      Set liburing version. '0' to skip.
  --jobs=${DFLT_NJOBS}
      Set number of parallel jobs.
  --arch=${DFLT_TARGETARCH}
      Set target architecture.
EOT
  exit 0
fi

NDNDPDK_DL_GITHUB=${NDNDPDK_DL_GITHUB:-https://github.com}
NDNDPDK_DL_GITHUB_API=${NDNDPDK_DL_GITHUB_API:-https://api.github.com}
NDNDPDK_DL_LLVM_APT=${NDNDPDK_DL_LLVM_APT:-https://apt.llvm.org}
NDNDPDK_DL_NODESOURCE_DEB=${NDNDPDK_DL_NODESOURCE_DEB:-https://deb.nodesource.com}
NDNDPDK_DL_PYPA_BOOTSTRAP=${NDNDPDK_DL_PYPA_BOOTSTRAP:-https://bootstrap.pypa.io}
NDNDPDK_DL_GOLANG=${NDNDPDK_DL_GOLANG:-https://golang.org}
NDNDPDK_DL_DPDK_FAST=${NDNDPDK_DL_DPDK_FAST:-https://fast.dpdk.org}
NDNDPDK_DL_DPDK=${NDNDPDK_DL_DPDK:-https://dpdk.org}
# you can also set GOPROXY enviornment variable, which will be persisted

curl_test() {
  local SITE=${!1}
  if ! curl -sfL $SITE$2 >/dev/null; then
    echo "Cannot reach ${SITE}"
    echo "You can specify a mirror site by setting $1 environ"
    echo "Example: $1=${SITE}"
    exit 1
  fi
}
curl_test NDNDPDK_DL_GITHUB /robots.txt
curl_test NDNDPDK_DL_GITHUB_API /robots.txt
curl_test NDNDPDK_DL_LLVM_APT
curl_test NDNDPDK_DL_NODESOURCE_DEB
curl_test NDNDPDK_DL_PYPA_BOOTSTRAP
curl_test NDNDPDK_DL_GOLANG /VERSION
curl_test NDNDPDK_DL_DPDK_FAST
curl_test NDNDPDK_DL_DPDK

github_resolve_commit() {
  local COMMIT=$1
  local REPO=$2
  if [[ ${#COMMIT} -ne 40 ]] && which jq >/dev/null; then
    curl -sfL ${NDNDPDK_DL_GITHUB_API}/repos/${REPO}/commits/${COMMIT} | jq -r '.sha'
  else
    echo ${COMMIT}
  fi
}

DISTRO=$(lsb_release -sc)
case $DISTRO in
  (bionic) ;;
  (focal) ;;
  (bullseye) ;;
  (*)
    echo "Distro ${DISTRO} is not supported by this script."
    if [[ -z $SKIPDISTROCHECK ]]; then
      echo 'To skip this check, set SKIPDISTROCHECK=1 environ'
      exit 1
    fi
    ;;
esac

if [[ $(echo $KERNELVER | awk -F. '{ print ($1*1000+$2>=5004) }') -ne 1 ]] &&
   [[ -z $SKIPKERNELCHECK ]] && ! [[ -f /.dockerenv ]]; then
  echo 'Linux kernel 5.4 or newer is required'
  if [[ $DISTRO == 'bionic' ]]; then
    echo 'To upgrade kernel, run this command and reboot:'
    echo "  ${APTSUGGEST} linux-generic-hwe-18.04"
  fi
  echo 'To skip this check, set SKIPKERNELCHECK=1 environ'
  exit 1
fi

if [[ $HAS_KERNEL_HEADERS == '0' ]] && ! [[ -f /.dockerenv ]]; then
  echo "Will skip certain features due to missing kernel headers. To install:"
  if [[ $DISTRO == 'bullseye' ]]; then
    echo "  ${APTSUGGEST} linux-headers-amd64 linux-headers-${KERNELVER}-amd64"
  else
    echo "  ${APTSUGGEST} linux-generic linux-headers-${KERNELVER}"
  fi
fi

APT_PKGS=(
  build-essential
  clang-11
  clang-format-11
  doxygen
  git
  jq
  libaio-dev
  libc6-dev-i386
  libelf-dev
  libnuma-dev
  libpcap-dev
  libssl-dev
  liburcu-dev
  pkg-config
  python3-distutils
  uuid-dev
  yamllint
)

if [[ $DISTRO != 'bionic' ]]; then
  APT_PKGS+=(python-is-python3)
fi

echo "Will download to ${CODEROOT}"
echo 'Will install C compiler and build tools'

if [[ $NODEVER == '0' ]]; then
  if ! which node >/dev/null; then
    echo '--node=0 specified but `node` command is unavailable'
    exit 1
  fi
else
  echo "Will install Node ${NODEVER}"
fi

if [[ $GOVER == '0' ]]; then
  if ! which go >/dev/null; then
    echo '--go=0 specified but `go` command is unavailable'
    exit 1
  fi
else
  if [[ $GOVER == 'latest' ]]; then
    GOVER=$(curl -sfL ${NDNDPDK_DL_GOLANG}/VERSION?m=text)
  fi
  echo "Will install Go ${GOVER}"
fi
echo 'Will install Go linters and tools'

if [[ $UBPFVER == '0' ]]; then
  if ! [[ -f /usr/local/include/ubpf.h ]]; then
    echo '--ubpf=0 specified but uBPF headers are absent'
    exit 1
  fi
else
  UBPFVER=$(github_resolve_commit $UBPFVER iovisor/ubpf)
  echo "Will install uBPF ${UBPFVER}"
fi

if [[ $LIBBPFVER != '0' ]]; then
  LIBBPFVER=$(github_resolve_commit $LIBBPFVER libbpf/libbpf)
  echo "Will install libbpf ${LIBBPFVER}"
fi

if [[ $DPDKVER == '0' ]]; then
  if ! [[ -f /usr/local/include/rte_common.h ]]; then
    echo '--dpdk=0 specified but DPDK headers are absent'
    exit 1
  fi
else
  echo "Will install DPDK ${DPDKVER} for ${TARGETARCH} architecture"
fi

if [[ $KMODSVER != '0' ]]; then
  echo "Will install DPDK kernel modules ${KMODSVER}"
  APT_PKGS+=(kmod)
fi

if [[ $SPDKVER == '0' ]]; then
  if ! [[ -f /usr/local/include/spdk/version.h ]]; then
    echo '--spdk=0 specified but SPDK headers are absent'
    exit 1
  fi
else
  echo "Will install SPDK ${SPDKVER} for ${TARGETARCH} architecture"
fi

if [[ $URINGVER == '0' ]]; then
  if ! [[ -f /usr/local/include/liburing.h ]] && ! [[ -f /usr/include/liburing.h ]]; then
    echo '--liburing=0 specified but liburing is absent'
    exit 1
  fi
else
  URINGVER=$(github_resolve_commit $URINGVER axboe/liburing)
  echo "Will install liburing ${URINGVER}"
fi

echo "Will compile with ${NJOBS} parallel jobs"
echo 'Will delete conflicting versions if present'
if [[ $CONFIRM -ne 1 ]]; then
  read -p 'Press ENTER to continue or CTRL+C to abort '
fi

$SUDO mkdir -p $CODEROOT
$SUDO chown -R $(id -u):$(id -g) $CODEROOT

echo 'Dpkg::Options {
   "--force-confdef";
   "--force-confold";
}
APT::Install-Recommends "no";
APT::Install-Suggests "no";' | $SUDO tee /etc/apt/apt.conf.d/80custom >/dev/null
if [[ $DISTRO == 'bionic' ]] && ! [[ -f /etc/apt/sources.list.d/llvm-11.list ]]; then
  curl -sfL ${NDNDPDK_DL_LLVM_APT}/llvm-snapshot.gpg.key | $SUDO apt-key add -
  echo "deb ${NDNDPDK_DL_LLVM_APT}/bionic/ llvm-toolchain-bionic-11 main" | $SUDO tee /etc/apt/sources.list.d/llvm-11.list
fi
$SUDO apt-get -y -qq update
$SUDO sh -c 'DEBIAN_FRONTEND=noninteractive apt-get -y -qq dist-upgrade'

if [[ $NODEVER != '0' ]]; then
  curl -sfL ${NDNDPDK_DL_NODESOURCE_DEB}/setup_${NODEVER} | $SUDO bash -
  APT_PKGS+=(nodejs)
fi

APT_PKG_LIST="${APT_PKGS[@]}"
$SUDO sh -c "DEBIAN_FRONTEND=noninteractive apt-get -y -qq install ${APT_PKG_LIST}"
$SUDO npm install -g graphqurl

if [[ $DISTRO == 'bionic' ]]; then
  $SUDO update-alternatives --remove-all python || true
  $SUDO update-alternatives --install /usr/bin/python python /usr/bin/python3 1
fi
curl -sfL ${NDNDPDK_DL_PYPA_BOOTSTRAP}/get-pip.py | $SUDO python
$SUDO pip install -U meson ninja pyelftools

if [[ $GOVER != '0' ]]; then
  $SUDO rm -rf /usr/local/go
  curl -sfL ${NDNDPDK_DL_GOLANG}/dl/${GOVER}.linux-amd64.tar.gz | $SUDO tar -C /usr/local -xz
  export PATH=$HOME/go/bin:/usr/local/go/bin:$PATH
  if ! grep /usr/local/go/bin ~/.bashrc >/dev/null; then
    echo 'export PATH=$HOME/go/bin:/usr/local/go/bin:$PATH' >>~/.bashrc
  fi
  if [[ -n $GOPROXY ]]; then
    go env -w GOPROXY="$GOPROXY"
  fi
fi

if [[ $UBPFVER != '0' ]]; then
  UBPFVER=$(github_resolve_commit $UBPFVER iovisor/ubpf)
  cd $CODEROOT
  rm -rf ubpf-${UBPFVER}
  curl -sfL ${NDNDPDK_DL_GITHUB}/iovisor/ubpf/archive/${UBPFVER}.tar.gz | tar -xz
  cd ubpf-${UBPFVER}/vm
  make -j${NJOBS}
  $SUDO make install
fi

if [[ $LIBBPFVER != '0' ]]; then
  LIBBPFVER=$(github_resolve_commit $LIBBPFVER libbpf/libbpf)
  cd $CODEROOT
  rm -rf libbpf-${LIBBPFVER}
  curl -sfL ${NDNDPDK_DL_GITHUB}/libbpf/libbpf/archive/${LIBBPFVER}.tar.gz | tar -xz
  cd libbpf-${LIBBPFVER}/src
  sh -c "umask 0000 && make -j${NJOBS}"
  $SUDO find /usr/local/lib -name 'libbpf.*' -delete
  $SUDO sh -c "umask 0000 && make install PREFIX=/usr/local LIBDIR=/usr/local/lib"
  $SUDO install -d -m0755 /usr/local/include/linux
  $SUDO install -m0644 ../include/uapi/linux/* /usr/local/include/linux
  $SUDO ldconfig
fi

if [[ $DPDKVER != '0' ]]; then
  cd $CODEROOT
  rm -rf dpdk-${DPDKVER}
  curl -sfL ${NDNDPDK_DL_DPDK_FAST}/rel/dpdk-${DPDKVER}.tar.xz | tar -xJ
  cd dpdk-${DPDKVER}
  meson -Ddebug=true -Doptimization=3 -Dmachine=${TARGETARCH} -Dtests=false --libdir=lib build
  cd build
  ninja -j${NJOBS}
  $SUDO find /usr/local/lib -name 'librte_*' -delete
  $SUDO ninja install
  $SUDO find /usr/local/lib -name 'librte_*.a' -delete
  $SUDO ldconfig
fi

if [[ $KMODSVER != '0' ]]; then
  cd $CODEROOT
  rm -rf dpdk-kmods
  git clone ${NDNDPDK_DL_DPDK}/git/dpdk-kmods
  cd dpdk-kmods
  git -c advice.detachedHead=false checkout $KMODSVER
  cd linux/igb_uio
  make
  UIODIR=/lib/modules/${KERNELVER}/kernel/drivers/uio
  $SUDO install -d -m0755 $UIODIR
  $SUDO install -m0644 igb_uio.ko $UIODIR
  $SUDO depmod
fi

if [[ $SPDKVER != '0' ]]; then
  cd $CODEROOT
  rm -rf spdk-${SPDKVER}
  curl -sfL ${NDNDPDK_DL_GITHUB}/spdk/spdk/archive/v${SPDKVER}.tar.gz | tar -xz
  cd spdk-${SPDKVER}
  ./configure --target-arch=${TARGETARCH} --with-shared --with-dpdk=/usr/local \
    --disable-tests --disable-unit-tests --disable-examples --disable-apps \
    --without-crypto --without-fuse --without-isal --without-vhost
  make -j${NJOBS}
  $SUDO find /usr/local/lib -name 'libspdk_*' -delete
  $SUDO make install
  $SUDO find /usr/local/lib -name 'libspdk_*.a' -delete
  $SUDO ldconfig
fi

if [[ $URINGVER != '0' ]]; then
  URINGVER=$(github_resolve_commit $URINGVER axboe/liburing)
  cd $CODEROOT
  rm -rf liburing-${URINGVER}
  curl -sfL ${NDNDPDK_DL_GITHUB}/axboe/liburing/archive/${URINGVER}.tar.gz | tar -xz
  cd liburing-${URINGVER}
  ./configure --prefix=/usr/local
  make
  $SUDO find /usr/local/lib -name 'liburing.*' -delete
  $SUDO find /usr/local/share/man -name 'io_uring*' -delete
  $SUDO rm -rf /usr/local/include/liburing /usr/local/include/liburing.h
  $SUDO make install
  $SUDO ldconfig
fi

(
  cd /tmp
  go install honnef.co/go/tools/cmd/staticcheck@latest
)

package dpdk

/*
#cgo CFLAGS: -m64 -pthread -O3 -march=native -I/usr/local/include/dpdk
#cgo LDFLAGS: -L/usr/local/lib -ldpdk -ldl -lnuma

extern int go_lcoreLaunch(void*);

#include <dlfcn.h> // dlopen()
#include <stdlib.h> // free()
#include <rte_config.h>
#include <rte_eal.h>
#include <rte_launch.h>
#include <rte_lcore.h>
#include <rte_memory.h>
*/
import "C"
import (
	"fmt"
	"io/ioutil"
	"strings"
	"unsafe"
)

type LCore uint
type NumaSocket int

const NUMA_SOCKET_ANY = NumaSocket(C.SOCKET_ID_ANY)

type LCoreState int

const (
	LCORE_STATE_WAIT LCoreState = iota
	LCORE_STATE_RUNNING
	LCORE_STATE_FINISHED
)

func GetCurrentLCore() LCore {
	return LCore(C.rte_lcore_id())
}

func (lc LCore) IsValid() bool {
	return lc != LCore(C.LCORE_ID_ANY)
}

func (lc LCore) IsMaster() bool {
	return C.rte_get_master_lcore() == C.uint(lc)
}

func (lc LCore) GetNumaSocket() NumaSocket {
	return NumaSocket(C.rte_lcore_to_socket_id(C.uint(lc)))
}

func (lc LCore) GetState() LCoreState {
	if !GetCurrentLCore().IsMaster() {
		panic("LCore.GetState is only available on master lcore")
	}
	return LCoreState(C.rte_eal_get_lcore_state(C.uint(lc)))
}

var lcoreFuncs = map[LCore]func() int{}

//export go_lcoreLaunch
func go_lcoreLaunch(lc unsafe.Pointer) C.int {
	return C.int(lcoreFuncs[LCore(uintptr(lc))]())
}

// Asynchonrously launch a function on an lcore.
// Returns whether success.
func (lc LCore) RemoteLaunch(f func() int) bool {
	if !GetCurrentLCore().IsMaster() {
		panic("LCore.RemoteLaunch is only available on master lcore")
	}
	lcoreFuncs[lc] = f
	res := C.rte_eal_remote_launch((*C.lcore_function_t)(C.go_lcoreLaunch),
		unsafe.Pointer(uintptr(lc)), C.uint(lc))
	return res == 0
}

// Wait for lcore to finish running, and return lcore function's return value.
// If lcore is not running, return 0 immediately.
func (lc LCore) Wait() int {
	if !GetCurrentLCore().IsMaster() {
		panic("LCore.Wait is only available on master lcore")
	}
	return int(C.rte_eal_wait_lcore(C.uint(lc)))
}

type Eal struct {
	Args   []string // remaining command-line arguments
	Master LCore
	Slaves []LCore
}

// Initialize DPDK Environment Abstraction Layer (EAL).
func NewEal(args []string) (*Eal, error) {
	e := loadDpdkDynLibs()
	if e != nil {
		return nil, e
	}
	eal := new(Eal)

	a := newCArgs(args)
	defer a.Close()

	res := int(C.rte_eal_init(a.Argc, a.Argv))
	if res < 0 {
		return nil, GetErrno()
	}
	eal.Args = a.GetRemainingArgs(res)

	eal.Master = LCore(C.rte_get_master_lcore())

	for i := C.rte_get_next_lcore(C.RTE_MAX_LCORE, 1, 1); i < C.RTE_MAX_LCORE; i = C.rte_get_next_lcore(i, 1, 0) {
		eal.Slaves = append(eal.Slaves, LCore(i))
	}

	return eal, nil
}

func loadDpdkDynLibs() error {
	const libdpdkPath = "/usr/local/lib/libdpdk.so"
	dpdkSoContent, e := ioutil.ReadFile(libdpdkPath)
	if e != nil {
		return e
	}

	dpdkText := strings.Split(string(dpdkSoContent), " ")
	if len(dpdkText) < 4 || dpdkText[0] != "GROUP" {
		return fmt.Errorf("unexpected text in %s", libdpdkPath)
	}

	for _, soname := range dpdkText[2 : len(dpdkText)-1] {
		cSoname := C.CString(soname)
		defer C.free(unsafe.Pointer(cSoname))
		h := C.dlopen(cSoname, C.RTLD_LAZY)
		if h == nil {
			return fmt.Errorf("dlopen failed for %s", soname)
		}
	}

	return nil
}

// Provide argc and argv to C code.
type cArgs struct {
	Argc    C.int            // argc for C code
	Argv    **C.char         // argv for C code
	strMems []unsafe.Pointer // C strings
}

func newCArgs(args []string) *cArgs {
	a := new(cArgs)
	a.Argc = C.int(len(args))

	var b *C.char
	ptrSize := unsafe.Sizeof(b)
	argv := C.malloc(C.size_t(ptrSize) * C.size_t(len(args)))
	a.Argv = (**C.char)(argv)

	for i, arg := range args {
		argvEle := (**C.char)(unsafe.Pointer(uintptr(argv) + uintptr(i)*ptrSize))
		*argvEle = C.CString(arg)
		a.strMems = append(a.strMems, unsafe.Pointer(*argvEle))
	}

	return a
}

// Get remaining argv tokens after the first nConsumed tokens have been consumed by C code.
func (a *cArgs) GetRemainingArgs(nConsumed int) []string {
	var b *C.char
	ptrSize := unsafe.Sizeof(b)

	rem := []string{}
	argv := uintptr(unsafe.Pointer(a.Argv))
	for i := nConsumed; i < int(a.Argc); i++ {
		argvEle := (**C.char)(unsafe.Pointer(uintptr(argv) + uintptr(i)*ptrSize))
		rem = append(rem, C.GoString(*argvEle))
	}

	return rem
}

func (a *cArgs) Close() {
	for _, strMem := range a.strMems {
		C.free(strMem)
	}
	C.free(unsafe.Pointer(a.Argv))
}
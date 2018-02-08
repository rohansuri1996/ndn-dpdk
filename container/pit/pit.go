package pit

/*
#include "../pcct/pit.h"
*/
import "C"
import (
	"unsafe"

	"ndn-dpdk/container/cs"
	"ndn-dpdk/container/pcct"
	"ndn-dpdk/ndn"
)

// The Pending Interest Table (PIT).
type Pit struct {
	*pcct.Pcct
}

func (pit Pit) getPtr() *C.Pit {
	return (*C.Pit)(pit.GetPtr())
}

func (pit Pit) Close() error {
	return nil
}

// Count number of PIT entries.
func (pit Pit) Len() int {
	return int(C.Pit_CountEntries(pit.getPtr()))
}

// Insert or find a PIT entry for the given Interest.
func (pit Pit) Insert(interest *ndn.InterestPkt) (pitEntry *Entry, csEntry *cs.Entry) {
	interestC := (*C.InterestPkt)(unsafe.Pointer(interest))
	insertRes := C.Pit_Insert(pit.getPtr(), interestC)
	switch C.PitInsertResult_GetKind(insertRes) {
	case C.PIT_INSERT_PIT:
		pitEntry = &Entry{C.PitInsertResult_GetPitEntry(insertRes)}
	case C.PIT_INSERT_CS:
		csEntry1 := cs.EntryFromPtr(unsafe.Pointer(C.PitInsertResult_GetCsEntry(insertRes)))
		csEntry = &csEntry1
	}
	return
}

// Erase a PIT entry.
func (pit Pit) Erase(entry Entry) {
	C.Pit_Erase(pit.getPtr(), entry.c)
	entry.c = nil
}

// Find a PIT entry for the given token.
func (pit Pit) Find(token uint64) *Entry {
	entryC := C.Pit_Find(pit.getPtr(), C.uint64_t(token))
	if entryC == nil {
		return nil
	}
	return &Entry{entryC}
}

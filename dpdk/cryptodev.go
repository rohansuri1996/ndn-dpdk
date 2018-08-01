package dpdk

/*
#include "cryptodev.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

type CryptoOpType int

const (
	CRYPTO_OP_SYM CryptoOpType = C.RTE_CRYPTO_OP_TYPE_SYMMETRIC
)

func (t CryptoOpType) String() string {
	switch t {
	case CRYPTO_OP_SYM:
		return "symmetric"
	}
	return fmt.Sprintf("%d", t)
}

type CryptoOpStatus int

const (
	CRYPTO_OP_SUCCESS  CryptoOpStatus = C.RTE_CRYPTO_OP_STATUS_SUCCESS
	CRYPTO_OP_NEW      CryptoOpStatus = C.RTE_CRYPTO_OP_STATUS_NOT_PROCESSED
	CRYPTO_OP_AUTHFAIL CryptoOpStatus = C.RTE_CRYPTO_OP_STATUS_AUTH_FAILED
	CRYPTO_OP_BADARG   CryptoOpStatus = C.RTE_CRYPTO_OP_STATUS_INVALID_ARGS
	CRYPTO_OP_ERROR    CryptoOpStatus = C.RTE_CRYPTO_OP_STATUS_ERROR
)

func (s CryptoOpStatus) String() string {
	switch s {
	case CRYPTO_OP_SUCCESS:
		return "success"
	case CRYPTO_OP_NEW:
		return "new"
	case CRYPTO_OP_AUTHFAIL:
		return "authfail"
	case CRYPTO_OP_BADARG:
		return "badarg"
	case CRYPTO_OP_ERROR:
		return "error"
	}
	return fmt.Sprintf("%d", s)
}

func (s CryptoOpStatus) Error() string {
	if s == CRYPTO_OP_SUCCESS {
		panic("not an error")
	}
	return fmt.Sprintf("CryptoOp-%s", s)
}

type CryptoOp struct {
	c *C.struct_rte_crypto_op
	// DO NOT add other fields: *CryptoOp is passed to C code as rte_crypto_op**
}

func (op CryptoOp) GetStatus() CryptoOpStatus {
	return CryptoOpStatus(C.CryptoOp_GetStatus(op.c))
}

func (op CryptoOp) PrepareSha256Digest(m Packet, offset, length int, output unsafe.Pointer) error {
	if offset < 0 || length < 0 || offset+length > m.Len() {
		return errors.New("offset+length exceeds packet boundary")
	}

	C.CryptoOp_PrepareSha256Digest(op.c, m.c, C.uint32_t(offset), C.uint32_t(length), (*C.uint8_t)(output))
	return nil
}

type CryptoOpPool struct {
	Mempool
}

func NewCryptoOpPool(name string, capacity int, cacheSize int, privSize int, socket NumaSocket) (mp CryptoOpPool, e error) {
	nameC := C.CString(name)
	defer C.free(unsafe.Pointer(nameC))

	mp.c = C.rte_crypto_op_pool_create(nameC, C.RTE_CRYPTO_OP_TYPE_UNDEFINED,
		C.uint(capacity), C.uint(cacheSize), C.uint16_t(privSize), C.int(socket))
	if mp.c == nil {
		return mp, GetErrno()
	}
	return mp, nil
}

func (mp CryptoOpPool) AllocBulk(opType CryptoOpType, ops []CryptoOp) error {
	ptr, count := ParseCptrArray(ops)
	res := C.rte_crypto_op_bulk_alloc(mp.c, C.enum_rte_crypto_op_type(opType),
		(**C.struct_rte_crypto_op)(ptr), C.uint16_t(count))
	if res == 0 {
		return errors.New("CryptoOp allocation failed")
	}
	return nil
}

type CryptoDev struct {
	devId       C.uint8_t
	sessionPool Mempool
}

func NewCryptoDev(name string, maxSessions, nQueuePairs int, socket NumaSocket) (cd CryptoDev, e error) {
	nameC := C.CString(name)
	defer C.free(unsafe.Pointer(nameC))
	if devId := C.rte_cryptodev_get_dev_id(nameC); devId < 0 {
		return CryptoDev{}, fmt.Errorf("cryptodev %s not found", name)
	} else {
		cd.devId = C.uint8_t(devId)
	}

	cd.sessionPool, e = NewMempool(name+"_sess", maxSessions*2, 0,
		int(C.rte_cryptodev_sym_get_private_session_size(cd.devId)), socket)
	if e != nil {
		return CryptoDev{}, e
	}

	var devConf C.struct_rte_cryptodev_config
	devConf.socket_id = C.int(socket)
	devConf.nb_queue_pairs = C.uint16_t(nQueuePairs)
	if res := C.rte_cryptodev_configure(cd.devId, &devConf); res < 0 {
		return CryptoDev{}, fmt.Errorf("rte_cryptodev_configure error %d", res)
	}

	var qpConf C.struct_rte_cryptodev_qp_conf
	qpConf.nb_descriptors = 2048
	for i := C.uint16_t(0); i < devConf.nb_queue_pairs; i++ {
		if res := C.rte_cryptodev_queue_pair_setup(cd.devId, i, &qpConf, C.int(socket),
			cd.sessionPool.c); res < 0 {
			return CryptoDev{}, fmt.Errorf("rte_cryptodev_queue_pair_setup(%d) error %d", i, res)
		}
	}

	if res := C.rte_cryptodev_start(cd.devId); res < 0 {
		return CryptoDev{}, fmt.Errorf("rte_cryptodev_start error %d", res)
	}

	return cd, nil
}

func (cd CryptoDev) Close() error {
	C.rte_cryptodev_stop(cd.devId)
	if res := C.rte_cryptodev_close(cd.devId); res < 0 {
		return fmt.Errorf("rte_cryptodev_close error %d", res)
	}
	return nil
}

func (cd CryptoDev) GetQueuePair(i int) (qp CryptoQueuePair, ok bool) {
	qp.CryptoDev = cd
	qp.qpId = C.uint16_t(i)
	if qp.qpId >= C.rte_cryptodev_queue_pair_count(cd.devId) {
		return CryptoQueuePair{}, false
	}
	return qp, true
}

type CryptoQueuePair struct {
	CryptoDev
	qpId C.uint16_t
}

func (qp CryptoQueuePair) EnqueueBurst(ops []CryptoOp) int {
	ptr, count := ParseCptrArray(ops)
	if count == 0 {
		return 0
	}
	res := C.rte_cryptodev_enqueue_burst(qp.devId, qp.qpId,
		(**C.struct_rte_crypto_op)(ptr), C.uint16_t(count))
	return int(res)
}

func (qp CryptoQueuePair) DequeueBurst(ops []CryptoOp) int {
	ptr, count := ParseCptrArray(ops)
	if count == 0 {
		return 0
	}
	res := C.rte_cryptodev_dequeue_burst(qp.devId, qp.qpId,
		(**C.struct_rte_crypto_op)(ptr), C.uint16_t(count))
	return int(res)
}

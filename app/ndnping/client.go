package ndnping

/*
#include "client.h"
#include "token.h"
*/
import "C"
import (
	"fmt"
	"math"
	"time"
	"unsafe"

	"ndn-dpdk/appinit"
	"ndn-dpdk/container/nameset"
	"ndn-dpdk/core/running_stat"
	"ndn-dpdk/dpdk"
	"ndn-dpdk/iface"
	"ndn-dpdk/ndn"
)

// Client internal config.
const (
	Client_BurstSize = 64
)

type Client struct {
	c *C.NdnpingClient
}

func newClient(face iface.IFace, cfg ClientConfig) (client Client) {
	socket := face.GetNumaSocket()
	client.c = (*C.NdnpingClient)(dpdk.Zmalloc("NdnpingClient", C.sizeof_NdnpingClient, socket))
	client.c.face = (C.FaceId)(face.GetFaceId())
	client.c.interestInterval = C.float(float64(cfg.Interval) / float64(time.Millisecond))

	client.c.interestMp = (*C.struct_rte_mempool)(appinit.MakePktmbufPool(
		appinit.MP_INT, socket).GetPtr())
	client.c.interestMbufHeadroom = C.uint16_t(appinit.SizeofEthLpHeaders() + ndn.EncodeInterest_GetHeadroom())

	C.NdnpingClient_Init(client.c)
	for _, patternCfg := range cfg.Patterns {
		client.getPatterns().InsertWithZeroUsr(patternCfg.Prefix, int(C.sizeof_NdnpingClientPattern))
	}

	return client
}

func (client Client) Close() error {
	client.getPatterns().Close()
	dpdk.Free(client.c)
	return nil
}

func (client Client) GetFace() iface.IFace {
	return iface.Get(iface.FaceId(client.c.face))
}

func (client Client) getPatterns() nameset.NameSet {
	return nameset.FromPtr(unsafe.Pointer(&client.c.patterns))
}

func (client Client) RunTx() int {
	C.NdnpingClient_RunTx(client.c)
	return 0
}

func (client Client) RunRx() int {
	C.NdnpingClient_RunRx(client.c)
	return 0
}

type ClientPatternCounters struct {
	NInterests uint64
	NData      uint64
	NNacks     uint64

	NRttSamples uint64
	RttMin      time.Duration
	RttMax      time.Duration
	RttAvg      time.Duration
	RttStdev    time.Duration
}

func (cnt ClientPatternCounters) String() string {
	return fmt.Sprintf("%dI %dD(%0.2f%%) %dN(%0.2f%%) rtt=%0.3f/%0.3f/%0.3f/%0.3fms(%dsamp)",
		cnt.NInterests,
		cnt.NData, float64(cnt.NData)/float64(cnt.NInterests)*100.0,
		cnt.NNacks, float64(cnt.NNacks)/float64(cnt.NInterests)*100.0,
		float64(cnt.RttMin)/float64(time.Millisecond), float64(cnt.RttAvg)/float64(time.Millisecond),
		float64(cnt.RttMax)/float64(time.Millisecond), float64(cnt.RttStdev)/float64(time.Millisecond),
		cnt.NRttSamples)
}

type ClientCounters struct {
	PerPattern  []ClientPatternCounters
	NInterests  uint64
	NData       uint64
	NNacks      uint64
	NAllocError uint64
}

func (cnt ClientCounters) String() string {
	s := fmt.Sprintf("%dI %dD(%0.2f%%) %dN(%0.2f%%) %dalloc-error", cnt.NInterests,
		cnt.NData, float64(cnt.NData)/float64(cnt.NInterests)*100.0,
		cnt.NNacks, float64(cnt.NNacks)/float64(cnt.NInterests)*100.0,
		cnt.NAllocError)
	for i, pcnt := range cnt.PerPattern {
		s += fmt.Sprintf(", pattern(%d) %s", i, pcnt)
	}
	return s
}

func (client Client) ReadCounters() (cnt ClientCounters) {
	durationUnit := dpdk.GetNanosInTscUnit() *
		math.Pow(2.0, float64(C.NDNPING_TIMING_PRECISION))
	toDuration := func(d float64) time.Duration {
		return time.Duration(d * durationUnit)
	}

	patterns := client.getPatterns()
	cnt.PerPattern = make([]ClientPatternCounters, patterns.Len())
	for i := 0; i < len(cnt.PerPattern); i++ {
		pattern := (*C.NdnpingClientPattern)(patterns.GetUsr(i))
		rtt := running_stat.FromPtr(unsafe.Pointer(&pattern.rtt))
		perPattern := ClientPatternCounters{
			NInterests:  uint64(pattern.nInterests),
			NData:       uint64(pattern.nData),
			NNacks:      uint64(pattern.nNacks),
			NRttSamples: rtt.Len64(),
			RttMin:      toDuration(rtt.Min()),
			RttMax:      toDuration(rtt.Max()),
			RttAvg:      toDuration(rtt.Mean()),
			RttStdev:    toDuration(rtt.Stdev()),
		}
		cnt.PerPattern[i] = perPattern
		cnt.NInterests += perPattern.NInterests
		cnt.NData += perPattern.NData
		cnt.NNacks += perPattern.NNacks
	}

	cnt.NAllocError = uint64(client.c.nAllocError)
	return cnt
}

package fwdptest

import (
	"testing"
	"time"

	"ndn-dpdk/app/fwdp/fwdptestfixture"
	"ndn-dpdk/ndn"
	"ndn-dpdk/ndn/ndntestutil"
)

func TestInterestData(t *testing.T) {
	assert, require := makeAR(t)
	fixture := fwdptestfixture.New(t)
	defer fixture.Close()

	face1 := fixture.CreateFace()
	face2 := fixture.CreateFace()
	face3 := fixture.CreateFace()
	fixture.SetFibEntry("/B", "multicast", face2.GetFaceId())
	fixture.SetFibEntry("/C", "multicast", face3.GetFaceId())

	interest := ndntestutil.MakeInterest("/B/1")
	ndntestutil.SetPitToken(interest, 0x0290dd7089e9d790)
	face1.Rx(interest)
	time.Sleep(100 * time.Millisecond)
	require.Len(face2.TxInterests, 1)
	assert.Len(face3.TxInterests, 0)

	data := ndntestutil.MakeData("/B/1")
	ndntestutil.CopyPitToken(data, face2.TxInterests[0])
	face2.Rx(data)
	time.Sleep(100 * time.Millisecond)
	require.Len(face1.TxData, 1)
	assert.Len(face1.TxNacks, 0)
	assert.Equal(ndntestutil.GetPitToken(face1.TxData[0]), uint64(0x0290dd7089e9d790))
}

func TestInterestDupNonce(t *testing.T) {
	assert, require := makeAR(t)
	fixture := fwdptestfixture.New(t)
	defer fixture.Close()

	face1 := fixture.CreateFace()
	face2 := fixture.CreateFace()
	face3 := fixture.CreateFace()
	fixture.SetFibEntry("/A", "multicast", face3.GetFaceId())

	interest := ndntestutil.MakeInterest("/A/1", uint32(0x6f937a51))
	ndntestutil.SetPitToken(interest, 0x3bddf54cffbc6ad0)
	face1.Rx(interest)
	time.Sleep(100 * time.Millisecond)
	assert.Len(face3.TxInterests, 1)

	interest = ndntestutil.MakeInterest("/A/1", uint32(0x6f937a51))
	ndntestutil.SetPitToken(interest, 0x3bddf54cffbc6ad0)
	face2.Rx(interest)
	time.Sleep(100 * time.Millisecond)
	require.Len(face3.TxInterests, 1)
	require.Len(face2.TxNacks, 1)
	assert.Equal(face2.TxNacks[0].GetReason(), ndn.NackReason_Duplicate)

	data := ndntestutil.MakeData("/A/1")
	ndntestutil.CopyPitToken(data, face3.TxInterests[0])
	face3.Rx(data)
	time.Sleep(100 * time.Millisecond)
	assert.Len(face1.TxData, 1)
	assert.Len(face1.TxNacks, 0)
	assert.Len(face2.TxData, 0)
	assert.Len(face2.TxNacks, 1)
}

func TestInterestSuppress(t *testing.T) {
	assert, _ := makeAR(t)
	fixture := fwdptestfixture.New(t)
	defer fixture.Close()

	face1 := fixture.CreateFace()
	face2 := fixture.CreateFace()
	face3 := fixture.CreateFace()
	fixture.SetFibEntry("/A", "multicast", face3.GetFaceId())

	go func() {
		ticker := time.NewTicker(1 * time.Millisecond)
		for i := 0; i < 400; i++ {
			<-ticker.C
			interest := ndntestutil.MakeInterest("/A/1")
			ndntestutil.SetPitToken(interest, 0xf4aab9f23eb5271e^uint64(i))
			if i%2 == 0 {
				face1.Rx(interest)
			} else {
				face2.Rx(interest)
			}
		}
		ticker.Stop()
	}()

	time.Sleep(500 * time.Millisecond)
	assert.Len(face3.TxInterests, 7)
	// suppression config is min=10, multiplier=2, max=100,
	// so Interests should be forwarded at 0, 10, 30, 70, 150, 250, 350
}

func TestInterestNoRoute(t *testing.T) {
	assert, require := makeAR(t)
	fixture := fwdptestfixture.New(t)
	defer fixture.Close()

	face1 := fixture.CreateFace()

	interestA1 := ndntestutil.MakeInterest("/A/1")
	ndntestutil.SetPitToken(interestA1, 0x431328d8b4075167)
	face1.Rx(interestA1)
	time.Sleep(100 * time.Millisecond)
	require.Len(face1.TxNacks, 1)
	assert.Equal(uint64(0x431328d8b4075167), ndntestutil.GetPitToken(face1.TxNacks[0]))
}

func TestCsHit(t *testing.T) {
	assert, require := makeAR(t)
	fixture := fwdptestfixture.New(t)
	defer fixture.Close()

	face1 := fixture.CreateFace()
	face2 := fixture.CreateFace()
	fixture.SetFibEntry("/B", "multicast", face2.GetFaceId())

	interestB1 := ndntestutil.MakeInterest("/B/1")
	ndntestutil.SetPitToken(interestB1, 0x193d673cdb9f85ac)
	face1.Rx(interestB1)
	time.Sleep(100 * time.Millisecond)
	require.Len(face2.TxInterests, 1)

	dataB1 := ndntestutil.MakeData("/B/1")
	ndntestutil.CopyPitToken(dataB1, face2.TxInterests[0])
	face2.Rx(dataB1)
	time.Sleep(100 * time.Millisecond)
	require.Len(face1.TxData, 1)
	assert.Equal(uint64(0x193d673cdb9f85ac), ndntestutil.GetPitToken(face1.TxData[0]))
	assert.Equal(0*time.Millisecond, face1.TxData[0].GetFreshnessPeriod())

	interestB1mbf := ndntestutil.MakeInterest("/B/1", ndn.MustBeFreshFlag)
	ndntestutil.SetPitToken(interestB1mbf, 0xf716737325e04a77)
	face1.Rx(interestB1mbf)
	time.Sleep(100 * time.Millisecond)
	require.Len(face2.TxInterests, 2)

	dataB1fp := ndntestutil.MakeData("/B/1", 2500*time.Millisecond)
	ndntestutil.CopyPitToken(dataB1fp, face2.TxInterests[1])
	face2.Rx(dataB1fp)
	time.Sleep(100 * time.Millisecond)
	require.Len(face1.TxData, 2)
	assert.Equal(uint64(0xf716737325e04a77), ndntestutil.GetPitToken(face1.TxData[1]))
	assert.Equal(2500*time.Millisecond, face1.TxData[1].GetFreshnessPeriod())

	interestB1 = ndntestutil.MakeInterest("/B/1")
	ndntestutil.SetPitToken(interestB1, 0xaec62dad2f669e6b)
	face1.Rx(interestB1)
	time.Sleep(100 * time.Millisecond)
	assert.Len(face2.TxInterests, 2)
	require.Len(face1.TxData, 3)
	assert.Equal(uint64(0xaec62dad2f669e6b), ndntestutil.GetPitToken(face1.TxData[2]))
	assert.Equal(2500*time.Millisecond, face1.TxData[2].GetFreshnessPeriod())

	interestB1mbf = ndntestutil.MakeInterest("/B/1")
	ndntestutil.SetPitToken(interestB1, 0xb5565a4e715c858d)
	face1.Rx(interestB1)
	time.Sleep(100 * time.Millisecond)
	assert.Len(face2.TxInterests, 2)
	require.Len(face1.TxData, 4)
	assert.Equal(uint64(0xb5565a4e715c858d), ndntestutil.GetPitToken(face1.TxData[3]))
	assert.Equal(2500*time.Millisecond, face1.TxData[3].GetFreshnessPeriod())
}

func TestFwHint(t *testing.T) {
	assert, require := makeAR(t)
	fixture := fwdptestfixture.New(t)
	defer fixture.Close()

	face1 := fixture.CreateFace()
	face2 := fixture.CreateFace()
	face3 := fixture.CreateFace()
	face4 := fixture.CreateFace()
	face5 := fixture.CreateFace()
	fixture.SetFibEntry("/A", "multicast", face1.GetFaceId())
	fixture.SetFibEntry("/B", "multicast", face2.GetFaceId())
	fixture.SetFibEntry("/C", "multicast", face3.GetFaceId())

	interest1 := ndntestutil.MakeInterest("/A/1", ndn.FHDelegation{1, "/B"}, ndn.FHDelegation{2, "/C"})
	ndntestutil.SetPitToken(interest1, 0x5c2fc6c972d830e7)
	face4.Rx(interest1)
	time.Sleep(10 * time.Millisecond)
	assert.Len(face1.TxInterests, 0)
	assert.Len(face2.TxInterests, 1)
	assert.Len(face3.TxInterests, 0)

	interest2 := ndntestutil.MakeInterest("/A/1", ndn.FHDelegation{1, "/C"}, ndn.FHDelegation{2, "/B"})
	ndntestutil.SetPitToken(interest2, 0x52e61e9eee7025b7)
	face5.Rx(interest2)
	time.Sleep(10 * time.Millisecond)
	assert.Len(face1.TxInterests, 0)
	require.Len(face2.TxInterests, 1)
	assert.Len(face3.TxInterests, 1)

	interest3 := ndntestutil.MakeInterest("/A/1", ndn.FHDelegation{1, "/Z"}, ndn.FHDelegation{2, "/B"})
	ndntestutil.SetPitToken(interest3, 0xa4291e2123c8211e)
	face5.Rx(interest3)
	time.Sleep(10 * time.Millisecond)
	assert.Len(face1.TxInterests, 0)
	assert.True(len(face2.TxInterests) <= 2)
	if len(face2.TxInterests) == 2 { // if retransmitting, second Interest should have same PitToken
		assert.Equal(ndntestutil.GetPitToken(face2.TxInterests[0]),
			ndntestutil.GetPitToken(face2.TxInterests[1]))
	}
	require.Len(face3.TxInterests, 1)

	data1 := ndntestutil.MakeData("/A/1", 1*time.Second) // satisfies interest1 and interest3
	ndntestutil.CopyPitToken(data1, face2.TxInterests[0])
	face2.Rx(data1)
	time.Sleep(10 * time.Millisecond)
	require.Len(face4.TxData, 1)
	assert.Equal(uint64(0x5c2fc6c972d830e7), ndntestutil.GetPitToken(face4.TxData[0]))
	assert.Equal(1*time.Second, face4.TxData[0].GetFreshnessPeriod())
	require.Len(face5.TxData, 1)
	assert.Equal(uint64(0xa4291e2123c8211e), ndntestutil.GetPitToken(face5.TxData[0]))
	assert.Equal(1*time.Second, face5.TxData[0].GetFreshnessPeriod())

	data2 := ndntestutil.MakeData("/A/1", 2*time.Second) // satisfies interest2
	ndntestutil.CopyPitToken(data2, face3.TxInterests[0])
	face3.Rx(data2)
	time.Sleep(10 * time.Millisecond)
	require.Len(face5.TxData, 2)
	assert.Equal(uint64(0x52e61e9eee7025b7), ndntestutil.GetPitToken(face5.TxData[1]))
	assert.Equal(2*time.Second, face5.TxData[1].GetFreshnessPeriod())

	interest4 := ndntestutil.MakeInterest("/A/1", ndn.FHDelegation{1, "/C"}) // matches data2
	ndntestutil.SetPitToken(interest4, 0xbb19e173f937f221)
	face4.Rx(interest4)
	time.Sleep(10 * time.Millisecond)
	require.Len(face4.TxData, 2)
	assert.Equal(uint64(0xbb19e173f937f221), ndntestutil.GetPitToken(face4.TxData[1]))
	assert.Equal(2*time.Second, face4.TxData[1].GetFreshnessPeriod())
}

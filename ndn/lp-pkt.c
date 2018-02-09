#include "lp-pkt.h"
#include "nack-pkt.h"
#include "tlv-encoder.h"

static bool
CanIgnoreLpHeader(uint64_t tlvType)
{
  return 800 <= tlvType && tlvType <= 959 && (tlvType & 0x3) == 0x0;
}

NdnError
DecodeLpPkt(TlvDecodePos* d, LpPkt* lpp)
{
  TlvElement lppEle;
  NdnError e = DecodeTlvElementExpectType(d, TT_LpPacket, &lppEle);
  RETURN_IF_UNLIKELY_ERROR;

  memset(lpp, 0, sizeof(LpPkt));
  lpp->fragCount = 1;

  TlvDecodePos d1;
  TlvElement_MakeValueDecoder(&lppEle, &d1);
  TlvElement hdrEle;
  while ((e = DecodeTlvElement(&d1, &hdrEle)) == NdnError_OK) {
    switch (hdrEle.type) {
      case TT_LpPayload:
        lpp->payloadOff = lppEle.size - hdrEle.length;
        TlvElement_MakeValueDecoder(&hdrEle, &lpp->payload);
        goto FOUND_PAYLOAD;
      case TT_LpSeqNo:
        // NDNLPv2 spec defines SeqNo as "fixed-width unsigned integer",
        // but ndn-cxx implements it as nonNegativeInteger.
        TlvElement_ReadNonNegativeInteger(&hdrEle, &lpp->seqNo);
        break;
      case TT_FragIndex: {
        uint64_t v;
        TlvElement_ReadNonNegativeInteger(&hdrEle, &v);
        if (v > UINT16_MAX) {
          return NdnError_LengthOverflow;
        }
        lpp->fragIndex = v;
        break;
      }
      case TT_FragCount: {
        uint64_t v;
        TlvElement_ReadNonNegativeInteger(&hdrEle, &v);
        if (v > UINT16_MAX) {
          return NdnError_LengthOverflow;
        }
        lpp->fragCount = v;
        break;
      }
      case TT_PitToken: {
        if (unlikely(hdrEle.length != 8)) {
          return NdnError_BadPitToken;
        }
        TlvDecodePos d2;
        TlvElement_MakeValueDecoder(&hdrEle, &d2);
        rte_le64_t v;
        MbufLoc_ReadU64(&d2, &v);
        lpp->pitToken = rte_le_to_cpu_64(v);
        break;
      }
      case TT_Nack: {
        TlvDecodePos d2;
        TlvElement_MakeValueDecoder(&hdrEle, &d2);
        TlvElement nackReasonEle;
        NdnError e2 =
          DecodeTlvElementExpectType(&d2, TT_NackReason, &nackReasonEle);
        if (unlikely(e2 == NdnError_Incomplete || e2 == NdnError_BadType)) {
          lpp->nackReason = NackReason_Unspecified;
          break;
        } else if (unlikely(e2 != NdnError_OK)) {
          return e2;
        }

        uint64_t v;
        TlvElement_ReadNonNegativeInteger(&nackReasonEle, &v);
        if (v > UINT8_MAX) {
          return NdnError_LengthOverflow;
        }
        lpp->nackReason = v;
        break;
      }
      case TT_CongestionMark: {
        uint64_t v;
        TlvElement_ReadNonNegativeInteger(&hdrEle, &v);
        if (v > UINT8_MAX) {
          return NdnError_LengthOverflow;
        }
        lpp->congMark = v;
        break;
      }
      default:
        if (!CanIgnoreLpHeader(hdrEle.type)) {
          return NdnError_UnknownCriticalLpHeader;
        }
        break;
    }
  }

FOUND_PAYLOAD:;
  if (unlikely(!MbufLoc_IsEnd(&d1))) {
    return e;
  }
  if (unlikely(lpp->fragIndex >= lpp->fragCount)) {
    return NdnError_FragIndexExceedFragCount;
  }
  return NdnError_OK;
}

void
EncodeLpHeaders(struct rte_mbuf* m, const LpPkt* lpp)
{
  assert(rte_pktmbuf_headroom(m) >= EncodeLpHeaders_GetHeadroom());
  assert(rte_pktmbuf_tailroom(m) >= EncodeLpHeaders_GetTailroom());
  assert(LpPkt_HasPayload(lpp));

  TlvEncoder* en = MakeTlvEncoder(m);

  if (LpPkt_IsFragmented(lpp)) {
    typedef struct FragHdr
    {
      char _padding[6]; // make TLV-VALUE fields aligned

      // NDNLPv2 spec defines SeqNo as a fixed-length field.
      uint8_t seqNoT;
      uint8_t seqNoL;
      rte_be64_t seqNoV;

      // FragIndex and FragCount are NonNegativeInteger fields, but NDN protocol does not
      // require NonNegativeInteger to use minimal length encoding.
      uint8_t fragIndexT;
      uint8_t fragIndexL;
      rte_be16_t fragIndexV;

      uint8_t fragCountT;
      uint8_t fragCountL;
      rte_be16_t fragCountV;
    } __rte_packed FragHdr;
    static_assert(sizeof(FragHdr) - offsetof(FragHdr, seqNoT) == 18, "");

    uint8_t* room = TlvEncoder_Append(en, 18);
    assert(room != NULL);
    FragHdr* hdr = (FragHdr*)(room - offsetof(FragHdr, seqNoT));

    hdr->seqNoT = TT_LpSeqNo;
    hdr->seqNoL = 8;
    hdr->seqNoV = rte_cpu_to_be_64(lpp->seqNo);
    hdr->fragIndexT = TT_FragIndex;
    hdr->fragIndexL = 2;
    hdr->fragIndexV = rte_cpu_to_be_16(lpp->fragIndex);
    hdr->fragCountT = TT_FragCount;
    hdr->fragCountL = 2;
    hdr->fragCountV = rte_cpu_to_be_16(lpp->fragCount);
  }

  if (lpp->fragIndex == 0) {
    if (lpp->pitToken != 0) {
      typedef struct PitTokenHdr
      {
        char _padding[6]; // make TLV-VALUE aligned
        uint8_t pitTokenT;
        uint8_t pitTokenL;
        rte_le64_t pitTokenV;
      } __rte_packed PitTokenHdr;
      static_assert(
        sizeof(PitTokenHdr) - offsetof(PitTokenHdr, pitTokenT) == 10, "");

      uint8_t* room = TlvEncoder_Append(en, 10);
      assert(room != NULL);
      PitTokenHdr* hdr =
        (PitTokenHdr*)(room - offsetof(PitTokenHdr, pitTokenT));

      hdr->pitTokenT = TT_PitToken;
      hdr->pitTokenL = 8;
      hdr->pitTokenV = rte_cpu_to_le_64(lpp->pitToken);
    }

    if (lpp->nackReason != NackReason_None) {
      AppendVarNum(en, TT_Nack);
      if (unlikely(lpp->nackReason == NackReason_Unspecified)) {
        AppendVarNum(en, 0);
      } else {
        AppendVarNum(en, 5);
        AppendVarNum(en, TT_NackReason);
        AppendVarNum(en, 1);
        *(TlvEncoder_Append(en, 1)) = lpp->nackReason;
      }
    }

    if (lpp->congMark != 0) {
      AppendVarNum(en, TT_CongestionMark);
      AppendVarNum(en, 1);
      *(TlvEncoder_Append(en, 1)) = lpp->congMark;
    }
  }

  if (m->data_len == 0) {
    // no LP header needed
    return;
  }

  AppendVarNum(en, TT_LpPayload);
  AppendVarNum(en, lpp->payload.rem);
  PrependVarNum(en, m->data_len + lpp->payload.rem);
  PrependVarNum(en, TT_LpPacket);
}

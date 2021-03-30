#ifndef NDNDPDK_BPF_XDP_API_H
#define NDNDPDK_BPF_XDP_API_H

/** @file */

#include "../../csrc/core/common.h"

#include <linux/bpf.h>

#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>

#include <linux/if_ether.h>

#define NDN_ETHERTYPE 0x8624

#define PacketPtrAs_(ptr, size, ...)                                                               \
  __extension__({                                                                                  \
    if ((const uint8_t*)ptr + (size_t)(size) > (const uint8_t*)(long)ctx->data_end) {              \
      return XDP_DROP;                                                                             \
    }                                                                                              \
    pkt;                                                                                           \
  })

/**
 * @brief Perform bounds-checking on packet pointer.
 *
 * This can be used within an XDP program, where `struct xdp_md* ctx` is declared.
 * If the structure dereferenced from the given pointer is within the bounds of the packet,
 * this returns the pointer; otherwise, the packet is dropped.
 *
 * @code
 * const Header* hdr = PacketPtrAs((const Header*)pkt);
 * const Header* hdr = PacketPtrAs((const Header*)pkt, HDR_LEN);
 * @endcode
 */
#define PacketPtrAs(ptr, ...) PacketPtrAs_((ptr), ##__VA_ARGS__, sizeof(*(ptr)))

#endif // NDNDPDK_BPF_XDP_API_H

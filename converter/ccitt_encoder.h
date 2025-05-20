
#ifndef CCITT_ENCODER_H
#define CCITT_ENCODER_H

#include <stddef.h>

int EncodeRawG4(
    unsigned char *bits1,
    size_t          width,
    size_t          height,
    unsigned char   **outBuf,
    unsigned long   *outSize
);

#endif
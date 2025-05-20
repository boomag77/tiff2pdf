#include <stdlib.h>
#include <string.h> 
#include <tiffio.h>

#include "settings.h"
#include "converter.h"


typedef struct {
    unsigned char *data;
    size_t         size;
    size_t         cap;
    toff_t         off;
} MemBuf;

// WriteProc для TIFFClientOpen
static tmsize_t writeCallback(thandle_t h, tdata_t buf, tsize_t sz) {
    MemBuf *m = (MemBuf*)h;
    toff_t newoff = m->off + sz;
    if (newoff > m->cap) {
        size_t newCap = (size_t)(newoff*2 + 1024);
        m->data = realloc(m->data, newCap);
        m->cap = newCap;
    }
    memcpy(m->data + m->off, buf, sz);
    m->off = newoff;
    if ((size_t)newoff > m->size) m->size = newoff;
    return sz;
}
// ReadProc for TIFFClientOpen — returns 0 bite
static tmsize_t readCallback(thandle_t h, tdata_t buf, tsize_t sz) {
    return 0;
}

// dummies callbacks
static toff_t sizeCallback(thandle_t h)   { return ((MemBuf*)h)->size; }
static int     closeCallback(thandle_t h)  { return 0; }
static toff_t seekCallback(thandle_t h, toff_t off, int whence) {
    MemBuf *m = (MemBuf*)h;
    toff_t newpos;
    switch(whence) {
        case SEEK_SET: newpos = off; break;
        case SEEK_CUR: newpos = m->off + off; break;
        case SEEK_END: newpos = m->size + off; break;
        default: return (toff_t)-1;
    }
    if (newpos < 0) return (toff_t)-1;
    m->off = newpos;
    return m->off;
}
static int mapCallback(thandle_t h, tdata_t* p, toff_t* n) {
  *p = NULL; *n = 0;
  return 0;
}
static void    unmapCallback(thandle_t h, tdata_t p, toff_t n) {}

// Encoding RAW G4-data (payload), without TIFF-header:
int encode_raw_g4(
    unsigned char *bits1,
    size_t          width,
    size_t          height,
    unsigned char   **outBuf,
    unsigned long   *outSize
) {
    MemBuf mb = { .data = NULL, .cap = 0, .size = 0, .off = 0 };

    // Open virtual TIFF in memory
    TIFF *tif = TIFFClientOpen(
        "g4", "w",
        (thandle_t)&mb,
        readCallback,
        writeCallback,
        seekCallback,
        closeCallback,
        sizeCallback,
        mapCallback,
        unmapCallback
    );
    if (!tif) {
        free(mb.data);
        return -1;
    }

    // set TIFF tags
    TIFFSetField(tif, TIFFTAG_IMAGEWIDTH,      width);
    TIFFSetField(tif, TIFFTAG_IMAGELENGTH,     height);
    TIFFSetField(tif, TIFFTAG_BITSPERSAMPLE,   1);
    TIFFSetField(tif, TIFFTAG_SAMPLESPERPIXEL, 1);
    TIFFSetField(tif, TIFFTAG_COMPRESSION,     COMPRESSION_CCITTFAX4);
    TIFFSetField(tif, TIFFTAG_PHOTOMETRIC,     PHOTOMETRIC_MINISWHITE);
    TIFFSetField(tif, TIFFTAG_FILLORDER,       FILLORDER_MSB2LSB);
    TIFFSetField(tif, TIFFTAG_ROWSPERSTRIP,    height);

    // ensure off == size
    seekCallback((thandle_t)&mb, 0, SEEK_END);
    unsigned long payloadStart = mb.size;

    // write one strip — libtiff perform G4-encoding
    tsize_t stripSize = TIFFWriteEncodedStrip(
        tif,
        0,
        bits1,
        ((width + 7) / 8) * height
    );
    TIFFClose(tif);

    if (stripSize < 0) {
        free(mb.data);
        return -2;
    }


    unsigned long payloadLen = mb.size - payloadStart;
    *outBuf  = (unsigned char*)malloc(payloadLen);
    if (!*outBuf) {
        free(mb.data);
        return -3;
    }
    memcpy(*outBuf, mb.data + payloadStart, payloadLen);
    *outSize = payloadLen;

    free(mb.data);
    return 0;
}
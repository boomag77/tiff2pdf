#include <stdlib.h>
#include <string.h>
#include <tiffio.h>

#include "settings.h"

#include "converter.h"


int extract_ccitt_raw(const char*     path,
                    unsigned char** outBuf,
                    unsigned long*  outSize,
                    size_t*         width,
                    size_t*         height)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

    // Get width and height
    size_t w=0, h=0;
    TIFFGetField(tif, TIFFTAG_IMAGEWIDTH,  &w);
    TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &h);

    // Ensure that it's CCITT-G4
    uint16_t comp=0;
    TIFFGetField(tif, TIFFTAG_COMPRESSION, &comp);
    if (comp != COMPRESSION_CCITTFAX4) {
        TIFFClose(tif);
        return -2;
    }

    // counting strips
    int nstrips = TIFFNumberOfStrips(tif);

    // Total size raw-lines
    toff_t total = 0;
    for (int i = 0; i < nstrips; i++) {
        total += TIFFRawStripSize(tif, i);
    }


    unsigned char* buf = malloc(total);
    if (!buf) {
        TIFFClose(tif);
        return -3;
    }
    toff_t offset = 0;
    for (int i = 0; i < nstrips; i++) {
        int sz = TIFFRawStripSize(tif, i);
        TIFFReadRawStrip(tif, i, buf + offset, sz);
        offset += sz;
    }

    TIFFClose(tif);

    *outBuf  = buf;
    *outSize = total;
    *width   = w;
    *height  = h;
    return 0;
}
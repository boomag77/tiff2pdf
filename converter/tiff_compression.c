
#include <stdlib.h>
#include <tiffio.h>

#include "converter.h"

int get_compression_type(const char* path) {
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;
    uint16_t compression = 0;
    TIFFGetField(tif, TIFFTAG_COMPRESSION, &compression);
    TIFFClose(tif);
    return (int)compression;
}
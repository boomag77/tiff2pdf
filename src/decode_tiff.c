#include <tiffio.h>


int decode_tiff_to_jpeg_c(const char* filename, int page, 
    unsigned char** outBuf, unsigned long* outSize,
    int quality, int dpi)
{
    TIFF* tif = TIFFOpen(filename, "r");
    if (!tif) return -1;

    if (page > 0 && TIFFSetDirectory(tif, page) == 0) {
        TIFFClose(tif);
        return -1;
    }

    uint32_t w=0, h=0;
    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &w) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &h)) {
        TIFFClose(tif);
        return -1;
    }

    uint32_t npix = w * h;
    uint32_t* raster = _TIFFmalloc(npix * sizeof(*raster));
    if (!raster) { TIFFClose(tif); return -1; }

    if (!TIFFReadRGBAImageOriented(tif, w, h, raster, ORIENTATION_TOPLEFT, 0)) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -1;
    }

    uint8_t* rgb = malloc(npix * 3);
    if (!rgb) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -1;
    }

    for (uint32_t i = 0; i < npix; i++) {
        uint32_t px = raster[i];
        rgb[3*i+0] = TIFFGetR(px);
        rgb[3*i+1] = TIFFGetG(px);
        rgb[3*i+2] = TIFFGetB(px);
    }

    int rc = write_jpeg_to_mem(w, h, rgb, quality, dpi, outBuf, outSize);

    free(rgb);
    _TIFFfree(raster);
    TIFFClose(tif);
    return rc == 0 ? 0 : -1;
}

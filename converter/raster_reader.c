#include <stdlib.h>
#include <tiffio.h>

#include "converter.h"



// Read TIFF raster
int read_raster(const char* path,
                uint32_t** raster, uint16_t* orig_dpi, size_t* orig_width, size_t* orig_height)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

    TIFFSetWarningHandler(NULL);

    size_t width = 0, height = 0;

    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &width) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &height)) {
        TIFFClose(tif);
        return -2;
    }

    float xres = 0.0f, yres = 0.0f;
    uint16_t resUnit = RESUNIT_NONE;
    *orig_dpi = 0;

    if (TIFFGetField(tif, TIFFTAG_XRESOLUTION, &xres) &&
        TIFFGetField(tif, TIFFTAG_YRESOLUTION, &yres) &&
        TIFFGetField(tif, TIFFTAG_RESOLUTIONUNIT, &resUnit)) {
        if (resUnit == RESUNIT_INCH) {
            *orig_dpi = (uint16_t)(xres + 0.5f);
        } else if (resUnit == RESUNIT_CENTIMETER) {
            *orig_dpi = (uint16_t)(xres * 2.54f + 0.5f);
        }
    }

    if (width != 0 && height > SIZE_MAX / width) {
        TIFFClose(tif);
        return -3;
    }

    *orig_width = width;
    *orig_height = height;

    *raster = malloc(width * height * sizeof(uint32_t));
    if (!*raster) {
        TIFFClose(tif);
        return -4;
    }

    if (!TIFFReadRGBAImageOriented(tif, width, height, *raster, ORIENTATION_TOPLEFT, 0)) {
        free(*raster);
        TIFFClose(tif);
        return -5;
    }

    TIFFClose(tif);
    return 0;
}
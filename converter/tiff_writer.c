

#include <stdlib.h>
#include <tiffio.h>

#include <tiff_writer.h>


void write_tiff(const char* filename,
                    uint32_t width, uint32_t height,
                    unsigned char* buf, size_t buf_size,
                    int dpi, int compression, int gray)
{
    TIFF* out = TIFFOpen(filename, "w");
    if (!out) return;

    TIFFSetField(out, TIFFTAG_IMAGEWIDTH, width);
    TIFFSetField(out, TIFFTAG_IMAGELENGTH, height);
    TIFFSetField(out, TIFFTAG_COMPRESSION, compression);
    TIFFSetField(out, TIFFTAG_XRESOLUTION, (float)dpi);
    TIFFSetField(out, TIFFTAG_YRESOLUTION, (float)dpi);
    TIFFSetField(out, TIFFTAG_RESOLUTIONUNIT, RESUNIT_INCH);


    if (compression == COMPRESSION_CCITTFAX4)
    {
        TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISWHITE);
        TIFFSetField(out, TIFFTAG_FILLORDER, FILLORDER_MSB2LSB);
        TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, height);
        TIFFSetField(out, TIFFTAG_BITSPERSAMPLE, 1);
        TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 1);

        TIFFWriteRawStrip(out, 0, buf, (tmsize_t)buf_size);
    }
        else if (compression == COMPRESSION_JPEG)
    {
        if (gray) {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISBLACK);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 1);
        } else {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_RGB);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 3);
        }
        TIFFSetField(out, TIFFTAG_BITSPERSAMPLE,   8);

        TIFFSetField(out, TIFFTAG_PLANARCONFIG,    PLANARCONFIG_CONTIG);

        TIFFSetField(out, TIFFTAG_JPEGQUALITY,  90);
        TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, height);
        TIFFWriteEncodedStrip(out, 0, (tdata_t)buf, (tmsize_t)(width * height * (gray ? 1 : 3)));

    } else {
        if (gray) {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISBLACK);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 1);
        } else {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_RGB);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 3);
        }
        TIFFSetField(out, TIFFTAG_BITSPERSAMPLE,   8);
        TIFFSetField(out, TIFFTAG_PLANARCONFIG,    PLANARCONFIG_CONTIG);
        TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, height);
        TIFFWriteEncodedStrip(out, 0, (tdata_t)buf, (tmsize_t)(width * height * (gray ? 1 : 3)));
    }
    TIFFClose(out);
}
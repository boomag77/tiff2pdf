#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>
#include <stdint.h>
#include <unistd.h>

int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer, int quality, int dpi, unsigned char** out, unsigned long* outSize) {
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    JSAMPROW row_pointer[1];
    int row_stride = width * 3;

    cinfo.err = jpeg_std_error(&jerr);
    jpeg_create_compress(&cinfo);
    jpeg_mem_dest(&cinfo, out, outSize);

    cinfo.image_width = width;
    cinfo.image_height = height;
    cinfo.input_components = 3;
    cinfo.in_color_space = JCS_RGB;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, quality, TRUE);
    cinfo.density_unit = 1;
    cinfo.X_density = dpi;
    cinfo.Y_density = dpi;

    jpeg_start_compress(&cinfo, TRUE);
    while (cinfo.next_scanline < cinfo.image_height) {
        row_pointer[0] = &buffer[cinfo.next_scanline * row_stride];
        jpeg_write_scanlines(&cinfo, row_pointer, 1);
    }
    jpeg_finish_compress(&cinfo);
    jpeg_destroy_compress(&cinfo);

    return 0;
}

int main(int argc, char* argv[]) {
    int quality = 90; // default

    // Считываем аргумент --quality=N
    if (argc == 2 && strncmp(argv[1], "--quality=", 10) == 0) {
        quality = atoi(argv[1] + 10);
        if (quality < 1) quality = 1;
        if (quality > 100) quality = 100;
    }

    fprintf(stderr, "Using JPEG quality: %d\n", quality);
    fflush(stderr);


    char tiffPath[4096];

    while (fgets(tiffPath, sizeof(tiffPath), stdin)) {
        tiffPath[strcspn(tiffPath, "\n")] = 0;

        TIFF* tif = TIFFOpen(tiffPath, "r");
        if (!tif) {
            fprintf(stderr, "Cannot open TIFF: %s\n", tiffPath);
            fflush(stderr);
            continue;
        }

        uint32_t width, height;
        TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &width);
        TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &height);

        float xres = 72.0;
        uint16_t res_unit = RESUNIT_INCH;
        TIFFGetField(tif, TIFFTAG_XRESOLUTION, &xres);
        TIFFGetField(tif, TIFFTAG_RESOLUTIONUNIT, &res_unit);
        int dpi = (res_unit == RESUNIT_CENTIMETER) ? (int)(xres * 2.54) : (int)(xres);

        uint32_t npixels = width * height;
        uint32_t* raster = _TIFFmalloc(npixels * sizeof(uint32_t));
        if (!raster) {
            TIFFClose(tif);
            continue;
        }

        if (!TIFFReadRGBAImageOriented(tif, width, height, raster, ORIENTATION_TOPLEFT, 0)) {
            _TIFFfree(raster);
            TIFFClose(tif);
            continue;
        }

        uint8_t* rgb = malloc(npixels * 3);
        for (uint32_t i = 0; i < npixels; i++) {
            rgb[i*3+0] = TIFFGetR(raster[i]);
            rgb[i*3+1] = TIFFGetG(raster[i]);
            rgb[i*3+2] = TIFFGetB(raster[i]);
        }

        unsigned char* jpegBuf = NULL;
        unsigned long jpegSize = 0;

        // 👇 Используем прочитанный quality!
        write_jpeg_to_mem(width, height, rgb, quality, dpi, &jpegBuf, &jpegSize);

        // Output size prefix (4 bytes)
        uint32_t size = (uint32_t)jpegSize;
        fwrite(&size, sizeof(uint32_t), 1, stdout);
        fwrite(jpegBuf, 1, jpegSize, stdout);
        fflush(stdout);

        free(rgb);
        _TIFFfree(raster);
        TIFFClose(tif);
        free(jpegBuf);
    }

    return 0;
}

// This code reads a TIFF image from stdin, converts it to JPEG format, and writes the JPEG data to stdout.
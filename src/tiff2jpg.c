#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>

int write_jpeg_to_stdout(uint32_t width, uint32_t height, uint8_t* buffer, int quality) {
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    JSAMPROW row_pointer[1];
    int row_stride = width * 3;

    cinfo.err = jpeg_std_error(&jerr);
    jpeg_create_compress(&cinfo);
    jpeg_stdio_dest(&cinfo, stdout);

    cinfo.image_width = width;
    cinfo.image_height = height;
    cinfo.input_components = 3;
    cinfo.in_color_space = JCS_RGB;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, quality, TRUE);
    jpeg_start_compress(&cinfo, TRUE);

    while (cinfo.next_scanline < cinfo.image_height) {
        row_pointer[0] = &buffer[cinfo.next_scanline * row_stride];
        jpeg_write_scanlines(&cinfo, row_pointer, 1);
    }

    jpeg_finish_compress(&cinfo);
    jpeg_destroy_compress(&cinfo);

    return 0;
}

int parse_quality_arg(const char* arg) {
    if (strncmp(arg, "--quality=", 10) == 0) {
        int q = atoi(arg + 10);
        if (q < 1) q = 1;
        if (q > 100) q = 100;
        return q;
    }
    return 90; // default
}

int main(int argc, char* argv[]) {
    if (argc < 2 || argc > 3) {
        fprintf(stderr, "Usage: %s input.tif [--quality=N]\n", argv[0]);
        return 1;
    }

    const char* input = argv[1];
    int quality = 90;
    if (argc == 3) {
        quality = parse_quality_arg(argv[2]);
    }

    TIFF* tif = TIFFOpen(input, "r");
    if (!tif) {
        fprintf(stderr, "Cannot open input TIFF: %s\n", input);
        return 2;
    }

    uint32_t width, height;
    TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &width);
    TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &height);

    uint32_t npixels = width * height;
    uint32_t* raster = (uint32_t*) _TIFFmalloc(npixels * sizeof(uint32_t));
    if (!raster) {
        TIFFClose(tif);
        fprintf(stderr, "Could not allocate raster buffer\n");
        return 3;
    }

    if (!TIFFReadRGBAImageOriented(tif, width, height, raster, ORIENTATION_TOPLEFT, 0)) {
        fprintf(stderr, "Failed to read TIFF image\n");
        _TIFFfree(raster);
        TIFFClose(tif);
        return 4;
    }

    uint8_t* rgb = malloc(npixels * 3);
    for (uint32_t i = 0; i < npixels; i++) {
        uint32_t px = raster[i];
        rgb[i * 3 + 0] = TIFFGetR(px);
        rgb[i * 3 + 1] = TIFFGetG(px);
        rgb[i * 3 + 2] = TIFFGetB(px);
    }

    _TIFFfree(raster);
    TIFFClose(tif);

    int result = write_jpeg_to_stdout(width, height, rgb, quality);
    free(rgb);
    return result;
}

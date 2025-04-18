#include <stdio.h>
#include <stdlib.h>
#include <tiffio.h>
#include <png.h>

// Пишем PNG прямо в stdout
int write_png_to_stdout(uint32_t width, uint32_t height, uint8_t* buffer) {
    png_structp png = png_create_write_struct(PNG_LIBPNG_VER_STRING, NULL, NULL, NULL);
    if (!png) return 1;

    png_infop info = png_create_info_struct(png);
    if (!info) return 2;

    if (setjmp(png_jmpbuf(png))) return 3;

    png_init_io(png, stdout);
    png_set_IHDR(
        png, info, width, height,
        8, PNG_COLOR_TYPE_RGB, PNG_INTERLACE_NONE,
        PNG_COMPRESSION_TYPE_DEFAULT, PNG_FILTER_TYPE_DEFAULT
    );
    png_write_info(png, info);

    for (uint32_t y = 0; y < height; y++) {
        png_write_row(png, buffer + y * width * 3);
    }

    png_write_end(png, NULL);
    png_destroy_write_struct(&png, &info);
    return 0;
}

int main(int argc, char* argv[]) {
    if (argc != 2) {
        fprintf(stderr, "Usage: %s input.tif\n", argv[0]);
        return 1;
    }

    const char* input = argv[1];
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

    // 👇 Пишем PNG в stdout
    int result = write_png_to_stdout(width, height, rgb);
    free(rgb);

    return result;
}

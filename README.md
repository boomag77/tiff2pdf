# tiff2pdf

A custom Go-based utility for converting TIFF images to PDF or other TIFF formats with advanced options for compression, resolution, and file handling.

## Overview

`tiff2pdf` is a command-line tool designed for converting TIFF images into PDF documents or processing TIFF files. It supports various compression methods, advanced page formatting, and legacy compatibility options. The tool is optimized for batch processing and includes features for handling grayscale and CCITT compression.

## Features

- Convert TIFF images to PDF or other TIFF formats
- Support for multiple compression methods (e.g., CCITT G4, JPEG, LZW, and legacy old-style compression)
- Advanced options for resolution, JPEG quality, and grayscale conversion
- Batch processing of TIFF files from directories
- Flexible TIFF handling modes: replace, convert, or append
- Debugging and verbose output for troubleshooting

## Installation

### Using Go

```bash
go install github.com/boomag77/tiff2pdf@latest
```

### From Source

```bash
git clone https://github.com/boomag77/tiff2pdf.git
cd tiff2pdf
go build
```

## Usage

Basic usage:

```bash
tiff2pdf [options] -input <input_directory>
```

## Options

### Input/Output

- `-input <directory>`: Input directory containing TIFF files or folders with TIFF files.
- `-output <directory>`: Output directory for converted files. Multiple output directories can be specified.

### File Type

- `-type <pdf|tiff>`: Specify the output file type. Default is `pdf`.

### TIFF Mode (for TIFF output)

- `-tiffmode <replace|convert|append>`: Specify how TIFF files are handled:
  - `replace`: Replace original TIFF files.
  - `convert`: Create new TIFF files in the output directory.
  - `append`: Append converted TIFF files to the original files.

### Compression

- `-ccitt <on|off|auto>`: Enable CCITT G4 compression:
  - `on`: Always use CCITT G4 compression.
  - `off`: Do not use CCITT G4 compression.
  - `auto`: Use CCITT G4 compression if possible.

### Resolution and Quality

- `-rgbdpi <value>`: DPI for RGB images. Default is 300.
- `-grdpi <value>`: DPI for grayscale images. Default is 300.
- `-rgbq <value>`: JPEG quality (1-100) for RGB images. Default is 100.
- `-grq <value>`: JPEG quality (1-100) for grayscale images. Default is 100.

### Debugging

- `-debug`: Enable debug output for troubleshooting.

## Examples

### Convert TIFF to PDF with default settings

```bash
tiff2pdf -input /path/to/tiff/folder -output /path/to/output
```

### Convert TIFF to PDF with CCITT G4 compression

```bash
tiff2pdf -input /path/to/tiff/folder -output /path/to/output -ccitt on
```

### Convert TIFF to TIFF with append mode

```bash
tiff2pdf -input /path/to/tiff/folder -output /path/to/output -type tiff -tiffmode append
```

### Specify custom DPI and JPEG quality

```bash
tiff2pdf -input /path/to/tiff/folder -output /path/to/output -rgbdpi 600 -grdpi 600 -rgbq 90 -grq 90
```

## Contributing

Contributions are welcome! Please submit issues or pull requests on the [GitHub repository](https://github.com/boomag77/tiff2pdf).

## License

This project is licensed under the [MIT License](LICENSE).

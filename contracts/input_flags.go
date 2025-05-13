package contracts

type InputFlags struct {
	OutputDir       []string
	InputRootDir    string
	OutputFileType  string
	CCITT           string
	TIFFMode        string
	RGBdpi          int
	GrayDpi         int
	GrayJpegQuality int
	RGBJpegQuality  int
}

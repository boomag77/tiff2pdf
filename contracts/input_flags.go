package contracts

type InputFlags struct {
	InputRootDir    string
	OutputDir       []string
	RGBdpi          int
	GrayDpi         int
	GrayJpegQuality int
	RGBJpegQuality  int
}

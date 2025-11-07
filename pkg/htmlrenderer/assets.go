package htmlrenderer

import (
	_ "embed"
)

//go:embed renderer.mjs
var rendererMjs string

//go:embed index.js
var indexJs string

//go:embed node.exe
var nodeExe []byte

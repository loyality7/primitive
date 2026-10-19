//go:build js && wasm

// Command wasm exposes the engine to JavaScript as a global "primitive"
// object. It is single threaded, so it uses one worker.
package main

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"syscall/js"
	"time"

	"github.com/nfnt/resize"

	"github.com/loyality7/primitive/primitive"
	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/imageio"
	"github.com/loyality7/primitive/primitive/shape"
)

var model *primitive.Model

func fail(msg string) any {
	return map[string]any{"error": msg}
}

func toJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// init(bytes, workingSize, outputSize, palette, edge[, candidates, restarts]) -> {width, height} | {error}
func initModel(_ js.Value, args []js.Value) any {
	if len(args) < 5 {
		return fail("init needs 5 arguments")
	}
	buf := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(buf, args[0])
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return fail("could not read image: " + err.Error())
	}
	if n := uint(args[1].Int()); n > 0 {
		img = resize.Thumbnail(n, n, img, resize.Bilinear)
	}
	palette, err := colors.ParsePalette(args[3].String())
	if err != nil {
		return fail(err.Error())
	}
	bg := colors.MakeColor(imageio.AverageImageColor(img))
	model = primitive.NewModel(img, bg, args[2].Int(), 1, time.Now().UnixNano())
	model.SetPalette(palette)
	model.SetEdgeBias(args[4].Float())
	if len(args) >= 7 {
		model.SetEffort(args[5].Int(), args[6].Int())
	}
	return map[string]any{"width": model.Sw, "height": model.Sh}
}

// step(shapeType, alpha) -> {shapes, score} | {error}
func step(_ js.Value, args []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	model.Step(shape.Type(args[0].Int()), args[1].Int(), 0)
	return map[string]any{"shapes": len(model.Shapes), "score": model.Score}
}

// pixels() -> Uint8Array of RGBA, width*height*4 bytes
func pixels(_ js.Value, _ []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	src := model.Context.Image()
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Rect, src, image.Point{}, draw.Src)
	return toJS(dst.Pix)
}

// svg(animSeconds, blur) -> string
func svg(_ js.Value, args []js.Value) any {
	if model == nil || len(model.Shapes) == 0 {
		return fail("nothing to export yet")
	}
	model.AnimStep = args[0].Float()
	model.Blur = args[1].Float()
	return model.SVG()
}

// png() -> Uint8Array
func pngBytes(_ js.Value, _ []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	var out bytes.Buffer
	if err := png.Encode(&out, model.Context.Image()); err != nil {
		return fail(err.Error())
	}
	return toJS(out.Bytes())
}

func main() {
	api := map[string]any{
		"init":   js.FuncOf(initModel),
		"step":   js.FuncOf(step),
		"pixels": js.FuncOf(pixels),
		"svg":    js.FuncOf(svg),
		"png":    js.FuncOf(pngBytes),
	}
	js.Global().Set("primitive", js.ValueOf(api))
	select {} // keep the runtime alive for JS calls
}

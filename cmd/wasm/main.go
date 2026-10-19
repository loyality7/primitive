//go:build js && wasm

// Command wasm exposes the engines to JavaScript as a global "primitive"
// object. It is single threaded, so the shape engine uses one worker.
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
	"github.com/loyality7/primitive/primitive/quad"
	"github.com/loyality7/primitive/primitive/shape"
)

// Only one of these is set at a time: the last init call wins.
var (
	model  *primitive.Model // shapes
	qmodel *quad.Model      // quadtree
)

func fail(msg string) any {
	return map[string]any{"error": msg}
}

func toJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// current is the picture of whichever engine was initialised last.
func current() image.Image {
	if qmodel != nil {
		return qmodel.Image()
	}
	if model != nil {
		return model.Context.Image()
	}
	return nil
}

// decode reads the photo bytes in args[0] and scales it to args[1] pixels.
func decode(args []js.Value) (image.Image, string) {
	buf := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(buf, args[0])
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "could not read image: " + err.Error()
	}
	if n := uint(args[1].Int()); n > 0 {
		img = resize.Thumbnail(n, n, img, resize.Bilinear)
	}
	return img, ""
}

// init(bytes, workingSize, outputSize, palette, edge[, candidates, restarts]) -> {width, height} | {error}
func initModel(_ js.Value, args []js.Value) any {
	if len(args) < 5 {
		return fail("init needs 5 arguments")
	}
	img, msg := decode(args)
	if msg != "" {
		return fail(msg)
	}
	palette, err := colors.ParsePalette(args[3].String())
	if err != nil {
		return fail(err.Error())
	}
	bg := colors.MakeColor(imageio.AverageImageColor(img))
	qmodel = nil
	model = primitive.NewModel(img, bg, args[2].Int(), 1, time.Now().UnixNano())
	model.SetPalette(palette)
	model.SetEdgeBias(args[4].Float())
	if len(args) >= 7 {
		model.SetEffort(args[5].Int(), args[6].Int())
	}
	return map[string]any{"width": model.Sw, "height": model.Sh}
}

// initQuads(bytes, workingSize, outputSize[, border, circles]) -> {width, height, leaves} | {error}
func initQuads(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("initQuads needs 3 arguments")
	}
	img, msg := decode(args)
	if msg != "" {
		return fail(msg)
	}
	model = nil
	qmodel = quad.NewModel(img, args[2].Int())
	if len(args) >= 4 {
		qmodel.Border = args[3].Int()
	}
	if len(args) >= 5 {
		qmodel.Circles = args[4].Truthy()
	}
	return map[string]any{"width": qmodel.Sw, "height": qmodel.Sh, "leaves": qmodel.Leaves()}
}

// step(shapeType, alpha) -> {shapes, score} | {error}
func step(_ js.Value, args []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	model.Step(shape.Type(args[0].Int()), args[1].Int(), 0)
	return map[string]any{"shapes": len(model.Shapes), "score": model.Score}
}

// stepQuads(n) splits up to n times -> {splits, leaves, score, finished} | {error}
func stepQuads(_ js.Value, args []js.Value) any {
	if qmodel == nil {
		return fail("call initQuads first")
	}
	done := 0
	for i := args[0].Int(); i > 0; i-- {
		if !qmodel.Split() {
			break
		}
		done++
	}
	return map[string]any{"splits": done, "leaves": qmodel.Leaves(), "score": qmodel.Score(), "finished": done == 0}
}

// pixels() -> Uint8Array of RGBA, width*height*4 bytes
func pixels(_ js.Value, _ []js.Value) any {
	src := current()
	if src == nil {
		return fail("call init first")
	}
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Rect, src, image.Point{}, draw.Src)
	return toJS(dst.Pix)
}

// svg(animSeconds, blur) -> string. Blur only applies to shapes.
func svg(_ js.Value, args []js.Value) any {
	if qmodel != nil {
		qmodel.AnimStep = args[0].Float()
		return qmodel.SVG()
	}
	if model == nil || len(model.Shapes) == 0 {
		return fail("nothing to export yet")
	}
	model.AnimStep = args[0].Float()
	model.Blur = args[1].Float()
	return model.SVG()
}

// png() -> Uint8Array
func pngBytes(_ js.Value, _ []js.Value) any {
	src := current()
	if src == nil {
		return fail("call init first")
	}
	var out bytes.Buffer
	if err := png.Encode(&out, src); err != nil {
		return fail(err.Error())
	}
	return toJS(out.Bytes())
}

// gif(delay, lastDelay) -> Uint8Array. Delays are in hundredths of a second.
func gifBytes(_ js.Value, args []js.Value) any {
	var frames []image.Image
	switch {
	case qmodel != nil:
		frames = qmodel.Frames(0)
	case model != nil && len(model.Shapes) > 0:
		frames = model.Frames(0.001)
	default:
		return fail("nothing to export yet")
	}
	data, err := imageio.EncodeGIF(frames, args[0].Int(), args[1].Int())
	if err != nil {
		return fail(err.Error())
	}
	return toJS(data)
}

func main() {
	api := map[string]any{
		"init":      js.FuncOf(initModel),
		"initQuads": js.FuncOf(initQuads),
		"step":      js.FuncOf(step),
		"stepQuads": js.FuncOf(stepQuads),
		"pixels":    js.FuncOf(pixels),
		"svg":       js.FuncOf(svg),
		"png":       js.FuncOf(pngBytes),
		"gif":       js.FuncOf(gifBytes),
	}
	js.Global().Set("primitive", js.ValueOf(api))
	select {} // keep the runtime alive for JS calls
}

//go:build js && wasm

// Command wasm exposes the engines to JavaScript as a global "primitive"
// object. It is single threaded, so the shape engine uses one worker.
package main

import (
	"bytes"
	"fmt"
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
	"github.com/loyality7/primitive/primitive/lowpoly"
	"github.com/loyality7/primitive/primitive/quad"
	"github.com/loyality7/primitive/primitive/shape"
)

// Only one of these is set at a time: the last init call wins.
var (
	model  *primitive.Model // shapes
	qmodel *quad.Model      // quadtree, also used for live video
	lmodel *lowpoly.Model   // triangle mesh, for photos and live video

	liveIn  *image.RGBA // the camera frame, reused
	liveOut *image.RGBA // the drawn picture, reused

	liveShapes bool // model is the shapes engine kept alive between camera frames
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
	if lmodel != nil {
		return lmodel.Image()
	}
	if qmodel != nil {
		return qmodel.Image()
	}
	if model != nil {
		return model.Context.Image()
	}
	return nil
}

// styleQuad applies the cell style and palette that the page asks for. cell is
// "square", "circle" or "triangle" (a plain true means circle, false square),
// palette is a comma separated list of colors or empty.
func styleQuad(m *quad.Model, border int, cell js.Value, palette string) string {
	pal, err := colors.ParsePalette(palette)
	if err != nil {
		return err.Error()
	}
	m.Border, m.Palette = border, pal
	m.Circles, m.Triangles = false, false
	switch {
	case cell.Type() == js.TypeString && cell.String() == "triangle":
		m.Triangles = true
	case cell.Type() == js.TypeString && cell.String() == "circle", cell.Type() == js.TypeBoolean && cell.Bool():
		m.Circles = true
	}
	m.Restart()
	return ""
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

// init(bytes, workingSize, outputSize, palette, edge[, candidates, restarts, seed]) -> {width, height} | {error}
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
	qmodel, lmodel, liveShapes = nil, nil, false
	seed := time.Now().UnixNano()
	if len(args) >= 8 {
		seed = int64(args[7].Int())
	}
	model = primitive.NewModel(img, bg, args[2].Int(), 1, seed)
	model.SetPalette(palette)
	model.SetEdgeBias(args[4].Float())
	if len(args) >= 7 {
		model.SetEffort(args[5].Int(), args[6].Int())
	}
	return map[string]any{"width": model.Sw, "height": model.Sh}
}

// initQuads(bytes, workingSize, outputSize[, border, cell, palette]) -> {width, height, leaves} | {error}
func initQuads(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("initQuads needs 3 arguments")
	}
	img, msg := decode(args)
	if msg != "" {
		return fail(msg)
	}
	model, lmodel, liveShapes = nil, nil, false
	qmodel = quad.NewModel(img, args[2].Int())
	if len(args) >= 5 {
		palette := ""
		if len(args) >= 6 {
			palette = args[5].String()
		}
		if msg := styleQuad(qmodel, args[3].Int(), args[4], palette); msg != "" {
			qmodel = nil
			return fail(msg)
		}
	} else if len(args) >= 4 {
		qmodel.Border = args[3].Int()
	}
	return map[string]any{"width": qmodel.Sw, "height": qmodel.Sh, "leaves": qmodel.Leaves()}
}

// initFrame(rgba, width, height, outputSize, palette, edge, candidates, restarts, seed)
// starts the shapes engine from one camera frame (width*height*4 bytes from
// getImageData) -> {width, height} | {error}. The picture is then kept alive
// with setTarget, search and apply, once per frame.
func initFrame(_ js.Value, args []js.Value) any {
	if len(args) < 9 {
		return fail("initFrame needs 9 arguments")
	}
	w, h := args[1].Int(), args[2].Int()
	if w < 8 || h < 8 || w > 1024 || h > 1024 {
		return fail("the frame size must be between 8 and 1024 pixels")
	}
	frame := image.NewRGBA(image.Rect(0, 0, w, h))
	if n := args[0].Get("length").Int(); n != len(frame.Pix) {
		return fail(fmt.Sprintf("a frame needs %d bytes, got %d", len(frame.Pix), n))
	}
	js.CopyBytesToGo(frame.Pix, args[0])
	palette, err := colors.ParsePalette(args[4].String())
	if err != nil {
		return fail(err.Error())
	}
	bg := colors.MakeColor(imageio.AverageImageColor(frame))
	qmodel, lmodel, liveShapes = nil, nil, true
	model = primitive.NewModel(frame, bg, args[3].Int(), 1, int64(args[8].Int()))
	model.SetPalette(palette)
	model.SetEdgeBias(args[5].Float())
	model.SetEffort(args[6].Int(), args[7].Int())
	liveIn = image.NewRGBA(frame.Rect)
	return map[string]any{"width": w, "height": h}
}

// setTarget(rgba, smooth) hands the engine the next camera frame and recolors
// every shape for it -> {shapes, score} | {error}
func setTarget(_ js.Value, args []js.Value) any {
	if model == nil || !liveShapes || liveIn == nil {
		return fail("call initFrame first")
	}
	if n := args[0].Get("length").Int(); n != len(liveIn.Pix) {
		return fail(fmt.Sprintf("a frame needs %d bytes, got %d", len(liveIn.Pix), n))
	}
	js.CopyBytesToGo(liveIn.Pix, args[0])
	if err := model.SetTarget(liveIn, args[1].Float()); err != nil {
		return fail(err.Error())
	}
	return map[string]any{"shapes": len(model.Shapes), "score": model.Score}
}

// shapesPixels() -> Uint8Array: the picture at the size of the camera frame,
// which is much faster to get than the output sized canvas
func shapesPixels(_ js.Value, _ []js.Value) any {
	if model == nil {
		return fail("call initFrame first")
	}
	return toJS(model.Current.Pix)
}

// initLowpoly(bytes, workingSize, outputSize, border, palette, points) -> {width, height, triangles} | {error}
// makes a triangle mesh picture of a photo; it is done at once.
func initLowpoly(_ js.Value, args []js.Value) any {
	if len(args) < 6 {
		return fail("initLowpoly needs 6 arguments")
	}
	img, msg := decode(args)
	if msg != "" {
		return fail(msg)
	}
	pal, err := colors.ParsePalette(args[4].String())
	if err != nil {
		return fail(err.Error())
	}
	frame := image.NewRGBA(img.Bounds())
	draw.Draw(frame, frame.Rect, img, img.Bounds().Min, draw.Src)
	m := lowpoly.New(frame.Rect.Dx(), frame.Rect.Dy(), args[2].Int(), args[5].Int())
	m.Border, m.Palette = args[3].Int(), pal
	if err := m.SetFrame(frame, 0); err != nil {
		return fail(err.Error())
	}
	model, qmodel, lmodel = nil, nil, m
	return map[string]any{"width": m.Sw, "height": m.Sh, "triangles": m.Triangles()}
}

// liveInit(width, height, outputSize, border, cell[, palette, points]) -> {width, height} | {error}
// prepares one quadtree that liveFrame refills for every camera frame.
func liveInit(_ js.Value, args []js.Value) any {
	if len(args) < 5 {
		return fail("liveInit needs 5 arguments")
	}
	w, h := args[0].Int(), args[1].Int()
	if w < 8 || h < 8 || w > 2048 || h > 2048 {
		return fail("the frame size must be between 8 and 2048 pixels")
	}
	palette := ""
	if len(args) >= 6 {
		palette = args[5].String()
	}
	if args[4].Type() == js.TypeString && args[4].String() == "lowpoly" {
		pal, err := colors.ParsePalette(palette)
		if err != nil {
			return fail(err.Error())
		}
		points := 1500
		if len(args) >= 7 {
			points = args[6].Int()
		}
		lm := lowpoly.New(w, h, args[2].Int(), points)
		lm.Border, lm.Palette = args[3].Int(), pal
		model, qmodel, lmodel = nil, nil, lm
		liveIn = image.NewRGBA(image.Rect(0, 0, w, h))
		liveOut = image.NewRGBA(image.Rect(0, 0, lm.Sw, lm.Sh))
		return map[string]any{"width": lm.Sw, "height": lm.Sh}
	}
	m := quad.NewEmpty(w, h, args[2].Int())
	m.NoHistory = true // live frames are not animated, so skip recording the splits
	if msg := styleQuad(m, args[3].Int(), args[4], palette); msg != "" {
		return fail(msg)
	}
	model, qmodel, lmodel = nil, m, nil
	liveIn = image.NewRGBA(image.Rect(0, 0, w, h))
	liveOut = image.NewRGBA(image.Rect(0, 0, m.Sw, m.Sh))
	return map[string]any{"width": m.Sw, "height": m.Sh}
}

// liveFrame(rgba, smooth, splits) turns one camera frame (width*height*4 bytes
// from getImageData) into the picture -> {pixels, leaves, score} | {error}
func liveFrame(_ js.Value, args []js.Value) any {
	if lmodel != nil && liveIn != nil && liveIn.Rect.Dx() == lmodel.W && liveIn.Rect.Dy() == lmodel.H {
		if n := args[0].Get("length").Int(); n != len(liveIn.Pix) {
			return fail(fmt.Sprintf("a frame needs %d bytes, got %d", len(liveIn.Pix), n))
		}
		js.CopyBytesToGo(liveIn.Pix, args[0])
		if err := lmodel.SetFrame(liveIn, args[1].Float()); err != nil {
			return fail(err.Error())
		}
		lmodel.RenderInto(liveOut)
		return map[string]any{"pixels": toJS(liveOut.Pix), "leaves": lmodel.Triangles(), "score": 0}
	}
	if qmodel == nil || liveIn == nil || liveIn.Rect.Dx() != qmodel.W || liveIn.Rect.Dy() != qmodel.H {
		return fail("call liveInit first")
	}
	if n := args[0].Get("length").Int(); n != len(liveIn.Pix) {
		return fail(fmt.Sprintf("a frame needs %d bytes, got %d", len(liveIn.Pix), n))
	}
	js.CopyBytesToGo(liveIn.Pix, args[0])
	if err := qmodel.SetFrame(liveIn, args[1].Float()); err != nil {
		return fail(err.Error())
	}
	for i := args[2].Int(); i > 0 && qmodel.Split(); i-- {
	}
	qmodel.RenderInto(liveOut)
	return map[string]any{"pixels": toJS(liveOut.Pix), "leaves": qmodel.Leaves(), "score": qmodel.Score()}
}

// step(shapeType, alpha) -> {shapes, score} | {error}
func step(_ js.Value, args []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	model.Step(shape.Type(args[0].Int()), args[1].Int(), 0)
	return map[string]any{"shapes": len(model.Shapes), "score": model.Score}
}

// search(shapeType, alpha, restarts) looks for the next shape but does not add
// it -> {type, params, alpha, energy} | {error}. Several copies of the same
// picture can search at once; apply then adds the best result to all of them.
func search(_ js.Value, args []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	state := model.Search(shape.Type(args[0].Int()), args[1].Int(), args[2].Int())
	typ, params := shape.Encode(state.Shape)
	list := make([]any, len(params))
	for i, p := range params {
		list[i] = p
	}
	return map[string]any{"type": int(typ), "params": list, "alpha": state.Alpha, "energy": state.Energy()}
}

// apply(type, params, alpha[, cap]) adds a shape found by search and, with a
// cap, drops the least useful shapes beyond it -> {shapes, score} | {error}
func apply(_ js.Value, args []js.Value) any {
	if model == nil {
		return fail("call init first")
	}
	p := make([]float64, args[1].Length())
	for i := range p {
		p[i] = args[1].Index(i).Float()
	}
	sh, err := shape.Decode(model.Workers[0].Env, shape.Type(args[0].Int()), p)
	if err != nil {
		return fail(err.Error())
	}
	model.Add(sh, args[2].Int())
	if len(args) >= 4 {
		model.Prune(args[3].Int())
	}
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
	if lmodel != nil {
		return lmodel.SVG()
	}
	if qmodel != nil {
		qmodel.AnimStep = args[0].Float()
		if qmodel.NoHistory {
			qmodel.AnimStep = 0 // a live picture has no recorded splits to animate
		}
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
	if model != nil && liveShapes {
		model.RedrawContext() // live video skips the output canvas until it is needed
	}
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
	case model != nil && liveShapes:
		return fail("a live picture has no history to make a GIF from")
	case lmodel != nil:
		return fail("a triangle mesh picture has no history to make a GIF from")
	case qmodel != nil && qmodel.NoHistory:
		return fail("a live picture has no history to make a GIF from")
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

// gifFromFrames(rgba, width, height, count, delay, lastDelay) -> Uint8Array.
// rgba holds count frames of width*height*4 bytes one after the other, for
// example frames the page collected from the live camera.
func gifFromFrames(_ js.Value, args []js.Value) any {
	if len(args) < 6 {
		return fail("gifFromFrames needs 6 arguments")
	}
	w, h, n := args[1].Int(), args[2].Int(), args[3].Int()
	size := w * h * 4
	if w < 1 || h < 1 || n < 1 || args[0].Get("length").Int() != size*n {
		return fail(fmt.Sprintf("expected %d frames of %dx%d (%d bytes)", n, w, h, size*n))
	}
	buf := make([]byte, size*n)
	js.CopyBytesToGo(buf, args[0])
	frames := make([]image.Image, n)
	for i := range frames {
		frames[i] = &image.RGBA{Pix: buf[i*size : (i+1)*size], Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	}
	data, err := imageio.EncodeGIF(frames, args[4].Int(), args[5].Int())
	if err != nil {
		return fail(err.Error())
	}
	return toJS(data)
}

func main() {
	api := map[string]any{
		"init":         js.FuncOf(initModel),
		"initQuads":    js.FuncOf(initQuads),
		"step":         js.FuncOf(step),
		"search":       js.FuncOf(search),
		"apply":        js.FuncOf(apply),
		"stepQuads":    js.FuncOf(stepQuads),
		"initLowpoly":  js.FuncOf(initLowpoly),
		"initFrame":    js.FuncOf(initFrame),
		"setTarget":    js.FuncOf(setTarget),
		"shapesPixels": js.FuncOf(shapesPixels),
		"liveInit":     js.FuncOf(liveInit),
		"liveFrame":    js.FuncOf(liveFrame),
		"pixels":       js.FuncOf(pixels),
		"svg":          js.FuncOf(svg),
		"png":          js.FuncOf(pngBytes),
		"gif":          js.FuncOf(gifBytes),
		"gifFrames":    js.FuncOf(gifFromFrames),
	}
	js.Global().Set("primitive", js.ValueOf(api))
	select {} // keep the runtime alive for JS calls
}

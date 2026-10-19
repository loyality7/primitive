package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/loyality7/primitive/primitive"
	"github.com/loyality7/primitive/primitive/colors"
	"github.com/loyality7/primitive/primitive/imageio"
	"github.com/loyality7/primitive/primitive/quad"
	"github.com/loyality7/primitive/primitive/shape"
	"github.com/nfnt/resize"
)

var (
	Input      string
	Outputs    flagArray
	Background string
	Configs    shapeConfigArray
	Alpha      int
	InputSize  int
	OutputSize int
	Mode       int
	Workers    int
	Nth        int
	Repeat     int
	Seed       int64
	V, VV      bool
	Interval   int
	Anim       float64
	Palette    string
	Edge       float64
	Blur       float64
	Algo       string
	Border     int
	Cell       string
	Quality    string
)

// renderer is what both engines offer for writing the output files.
type renderer interface {
	Image() image.Image
	SVG() string
	Frames(scoreDelta float64) []image.Image
}

type flagArray []string

func (i *flagArray) String() string {
	return strings.Join(*i, ", ")
}

func (i *flagArray) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type shapeConfig struct {
	Count  int
	Mode   int
	Alpha  int
	Repeat int
}

type shapeConfigArray []shapeConfig

func (i *shapeConfigArray) String() string {
	return ""
}

func (i *shapeConfigArray) Set(value string) error {
	n, _ := strconv.ParseInt(value, 0, 0)
	*i = append(*i, shapeConfig{int(n), Mode, Alpha, Repeat})
	return nil
}

func init() {
	flag.StringVar(&Input, "i", "", "input image path")
	flag.Var(&Outputs, "o", "output image path")
	flag.Var(&Configs, "n", "number of shapes, or number of splits with -algo quads")
	flag.StringVar(&Background, "bg", "", "background color (hex)")
	flag.IntVar(&Alpha, "a", 128, "alpha value")
	flag.IntVar(&InputSize, "r", 256, "resize large input images to this size")
	flag.IntVar(&OutputSize, "s", 1024, "output image size")
	flag.IntVar(&Mode, "m", 1, "0=combo 1=triangle 2=rect 3=ellipse 4=circle 5=rotatedrect 6=beziers 7=rotatedellipse 8=polygon")
	flag.IntVar(&Workers, "j", 0, "number of parallel workers (default uses all cores)")
	flag.IntVar(&Nth, "nth", 1, "save every Nth frame (put \"%d\" in path)")
	flag.IntVar(&Repeat, "rep", 0, "add N extra shapes per iteration with reduced search")
	flag.Func("seed", "seed to feed the RNG", func(value string) (err error) {
		Seed, err = strconv.ParseInt(value, 10, 64)
		return
	})
	flag.BoolVar(&V, "v", false, "verbose")
	flag.BoolVar(&VV, "vv", false, "very verbose")
	flag.Float64Var(&Anim, "anim", 0, "animate SVG output: seconds between shapes (or splits) appearing (0 = static)")
	flag.StringVar(&Algo, "algo", "shapes", "shapes (default) or quads: a quadtree that keeps splitting the worst square into four")
	flag.IntVar(&Border, "border", 1, "quads only: width in output pixels of the dark outline around each square (0 = none)")
	flag.StringVar(&Cell, "cell", "square", "quads only: draw each cell as a square or a circle")
	flag.Float64Var(&Blur, "blur", 0, "SVG output only: gaussian blur radius in image pixels (0 = off)")
	flag.StringVar(&Quality, "quality", "", "preset for -n and -r: draft (150 shapes, r256) or high (400 shapes, r512); explicit -n / -r win")
	flag.Float64Var(&Edge, "edge", 0, "0-1: chance a shape is placed near high-detail areas (0 = uniform)")
	flag.StringVar(&Palette, "palette", "", "restrict colors to comma-separated hex list, e.g. \"#000000,#ff0000,#ffffff\"")
	flag.IntVar(&Interval, "int", 50, "interval between frame in a gif")
}

func errorMessage(message string) bool {
	fmt.Fprintln(os.Stderr, message)
	return false
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// parse and validate arguments
	flag.Parse()
	ok := true
	if Input == "" {
		ok = errorMessage("ERROR: input argument required")
	}
	if len(Outputs) == 0 {
		ok = errorMessage("ERROR: output argument required")
	}
	if Blur < 0 {
		ok = errorMessage("ERROR: blur must be >= 0")
	}
	if Quality != "" {
		presets := map[string][2]int{"draft": {150, 256}, "high": {400, 512}}
		p, found := presets[Quality]
		if !found {
			ok = errorMessage("ERROR: quality must be draft or high")
		} else {
			explicit := map[string]bool{}
			flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
			if len(Configs) == 0 {
				Configs = append(Configs, shapeConfig{p[0], Mode, Alpha, Repeat})
			}
			if !explicit["r"] {
				InputSize = p[1]
			}
		}
	}
	if len(Configs) == 0 {
		ok = errorMessage("ERROR: number argument required")
	}
	if len(Configs) == 1 {
		Configs[0].Mode = Mode
		Configs[0].Alpha = Alpha
		Configs[0].Repeat = Repeat
	}
	for _, config := range Configs {
		if config.Count < 1 {
			ok = errorMessage("ERROR: number argument must be > 0")
		}
	}
	if Algo != "shapes" && Algo != "quads" {
		ok = errorMessage("ERROR: algo must be shapes or quads")
	}
	if Cell != "square" && Cell != "circle" {
		ok = errorMessage("ERROR: cell must be square or circle")
	}
	if Border < 0 {
		ok = errorMessage("ERROR: border must be >= 0")
	}
	if Algo != "quads" {
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "border" || f.Name == "cell" {
				ok = errorMessage("ERROR: -" + f.Name + " only applies to -algo quads")
			}
		})
	}
	if Algo == "quads" {
		flag.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "m", "a", "rep", "bg", "palette", "edge", "blur", "seed", "j":
				ok = errorMessage("ERROR: -" + f.Name + " does not apply to -algo quads")
			}
		})
	}
	if Interval < 0 {
		ok = errorMessage("ERROR: negative frame interval not accepted")
	}
	if !ok {
		fmt.Println("Usage: primitive [OPTIONS] -i input -o output -n count")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// set log level
	if V {
		primitive.LogLevel = 1
	}
	if VV {
		primitive.LogLevel = 2
	}

	// seed random number generator
	if Seed == 0 {
		Seed = time.Now().UTC().UnixNano()
	}
	rand.Seed(Seed)

	// color palette
	palette, err := colors.ParsePalette(Palette)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}

	// determine worker count
	if Workers < 1 {
		Workers = runtime.NumCPU()
	}

	// read input image
	primitive.Log(1, "reading %s\n", Input)
	input, err := imageio.LoadImage(Input)
	check(err)

	// scale down input image if needed
	size := uint(InputSize)
	if size > 0 {
		input = resize.Thumbnail(size, size, input, resize.Bilinear)
	}

	// determine background color
	var bg colors.Color
	if Background == "" {
		bg = colors.MakeColor(imageio.AverageImageColor(input))
	} else {
		bg = colors.MakeHexColor(Background)
	}

	// run algorithm
	var out renderer
	var step func(config shapeConfig) int // does one unit of work, returns a count for the log
	var score func() float64
	if Algo == "quads" {
		qm := quad.NewModel(input, OutputSize)
		qm.AnimStep = Anim
		qm.Border = Border
		qm.Circles = Cell == "circle"
		out, score = qm, qm.Score
		step = func(shapeConfig) int {
			if qm.Split() {
				return 1
			}
			return 0
		}
	} else {
		model := primitive.NewModel(input, bg, OutputSize, Workers, Seed+1)
		model.AnimStep = Anim
		model.Blur = Blur
		model.SetPalette(palette)
		model.SetEdgeBias(Edge)
		out, score = model, func() float64 { return model.Score }
		step = func(config shapeConfig) int {
			return model.Step(shape.Type(config.Mode), config.Alpha, config.Repeat)
		}
	}
	primitive.Log(1, "%d: t=%.3f, score=%.6f\n", 0, 0.0, score())
	start := time.Now()
	frame := 0
	total := 0
	for _, config := range Configs {
		total += config.Count
	}
	for j, config := range Configs {
		primitive.Log(1, "count=%d, mode=%d, alpha=%d, repeat=%d\n",
			config.Count, config.Mode, config.Alpha, config.Repeat)

		for i := 0; i < config.Count; i++ {
			frame++

			// find optimal shape and add it to the model
			t := time.Now()
			n := step(config)
			nps := imageio.NumberString(float64(n) / time.Since(t).Seconds())
			elapsed := time.Since(start).Seconds()
			primitive.Log(1, "%d: t=%.3f, score=%.6f, n=%d, n/s=%s\n", frame, elapsed, score(), n, nps)

			if !V && !VV {
				progress(frame, total, start)
			}

			// write output image(s)
			for _, output := range Outputs {
				ext := strings.ToLower(filepath.Ext(output))
				if output == "-" {
					ext = ".svg"
				}
				percent := strings.Contains(output, "%")
				saveFrames := percent && ext != ".gif"
				saveFrames = saveFrames && frame%Nth == 0
				last := j == len(Configs)-1 && i == config.Count-1
				if saveFrames || last {
					path := output
					if percent {
						path = fmt.Sprintf(output, frame)
					}
					primitive.Log(1, "writing %s\n", path)
					switch ext {
					default:
						check(fmt.Errorf("unrecognized file extension: %s", ext))
					case ".png":
						check(imageio.SavePNG(path, out.Image()))
					case ".jpg", ".jpeg":
						check(imageio.SaveJPG(path, out.Image(), 95))
					case ".svg":
						check(imageio.SaveFile(path, out.SVG()))
					case ".gif":
						frames := out.Frames(0.001)
						check(imageio.SaveGIFImageMagick(path, frames, Interval, 250))
					}
				}
			}
		}
	}
}

// progress draws a one-line progress bar with ETA on stderr.
func progress(done, total int, start time.Time) {
	const width = 30
	filled := width * done / total
	eta := time.Duration(0)
	if done > 0 {
		eta = time.Duration(float64(time.Since(start)) / float64(done) * float64(total-done))
	}
	fmt.Fprintf(os.Stderr, "[%s%s] %d/%d  eta %s ", strings.Repeat("#", filled), strings.Repeat(".", width-filled), done, total, eta.Round(time.Second))
	if done == total {
		fmt.Fprintln(os.Stderr)
	}
}

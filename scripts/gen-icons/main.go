// Command gen-icons derives platform icons from the project's original JPEG.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	windows := flag.Bool("windows-resources", false, "regenerate Windows COFF icon resources using pinned rsrc")
	flag.Parse()
	root, err := os.Getwd()
	check(err)
	if filepath.Base(root) == "ico" {
		root = filepath.Dir(root)
	}
	directory := filepath.Join(root, "ico")
	file, err := os.Open(filepath.Join(directory, "reia-clash.jpeg"))
	check(err)
	source, err := jpeg.Decode(file)
	check(err)
	check(file.Close())
	images := map[int][]byte{}
	for _, size := range []int{16, 24, 32, 48, 64, 128, 256, 512, 1024} {
		var out bytes.Buffer
		check(png.Encode(&out, resize(source, size)))
		images[size] = out.Bytes()
	}
	check(os.WriteFile(filepath.Join(directory, "reia-clash.png"), images[256], 0644))
	check(os.WriteFile(filepath.Join(directory, "reia-clash-64.png"), images[64], 0644))
	var ico bytes.Buffer
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	writeLE(&ico, uint16(0), uint16(1), uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for _, size := range sizes {
		ico.Write([]byte{byte(size), byte(size), 0, 0})
		writeLE(&ico, uint16(1), uint16(32), uint32(len(images[size])), uint32(offset))
		offset += len(images[size])
	}
	for _, size := range sizes {
		ico.Write(images[size])
	}
	iconPath := filepath.Join(directory, "reia-clash.ico")
	check(os.WriteFile(iconPath, ico.Bytes(), 0644))
	htmlPath := filepath.Join(root, "test-webui.html")
	html, err := os.ReadFile(htmlPath)
	check(err)
	tag := `<link id="reia-favicon" rel="icon" type="image/png" href="data:image/png;base64,` + base64.StdEncoding.EncodeToString(images[64]) + `">`
	pattern := regexp.MustCompile(`<link id="reia-favicon"[^>]*>`)
	if pattern.Match(html) {
		html = pattern.ReplaceAll(html, []byte(tag))
	} else {
		html = []byte(strings.Replace(string(html), "</head>", tag+"\n</head>", 1))
	}
	check(os.WriteFile(htmlPath, html, 0644))
	var chunks bytes.Buffer
	for _, entry := range []struct {
		code string
		size int
	}{{"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}} {
		chunks.WriteString(entry.code)
		check(binary.Write(&chunks, binary.BigEndian, uint32(8+len(images[entry.size]))))
		chunks.Write(images[entry.size])
	}
	var icns bytes.Buffer
	icns.WriteString("icns")
	check(binary.Write(&icns, binary.BigEndian, uint32(8+chunks.Len())))
	icns.Write(chunks.Bytes())
	check(os.WriteFile(filepath.Join(directory, "reia-clash.icns"), icns.Bytes(), 0644))
	if *windows {
		buildDir := filepath.Join(root, ".build", "icon-resources")
		check(os.MkdirAll(buildDir, 0755))
		for _, arch := range []string{"386", "amd64", "arm64"} {
			output := filepath.Join(buildDir, "icon_windows_"+arch+".syso")
			cmd := exec.Command("go", "run", "github.com/akavel/rsrc@v0.10.2", "-arch", arch, "-ico", iconPath, "-o", output)
			cmd.Dir = root
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			check(cmd.Run())
			resource, err := os.ReadFile(output)
			check(err)
			for _, part := range []string{"reia", "reia-agent", "reia-tray", "reia-service"} {
				check(os.WriteFile(filepath.Join(root, "cmd", part, filepath.Base(output)), resource, 0644))
			}
		}
	}
	fmt.Println("ReiaClash icons generated from ico/reia-clash.jpeg")
}

func writeLE(out *bytes.Buffer, values ...any) {
	for _, value := range values {
		check(binary.Write(out, binary.LittleEndian, value))
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Bilinear sampling keeps the original artwork and aspect ratio. Letterbox
// non-square replacements instead of cropping the source.
func resize(source image.Image, size int) image.Image {
	output := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := source.Bounds()
	scale := math.Min(float64(size)/float64(bounds.Dx()), float64(size)/float64(bounds.Dy()))
	w, h := float64(bounds.Dx())*scale, float64(bounds.Dy())*scale
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := (float64(x)+0.5-(float64(size)-w)/2)/scale - 0.5
			fy := (float64(y)+0.5-(float64(size)-h)/2)/scale - 0.5
			if fx < -0.5 || fy < -0.5 || fx >= float64(bounds.Dx())-0.5 || fy >= float64(bounds.Dy())-0.5 {
				continue
			}
			fx = math.Max(0, math.Min(fx, float64(bounds.Dx()-1)))
			fy = math.Max(0, math.Min(fy, float64(bounds.Dy()-1)))
			x0, y0 := int(fx), int(fy)
			x1, y1 := min(x0+1, bounds.Dx()-1), min(y0+1, bounds.Dy()-1)
			dx, dy := fx-float64(x0), fy-float64(y0)
			var sums [4]float64
			for _, point := range []struct {
				x, y   int
				weight float64
			}{{x0, y0, (1 - dx) * (1 - dy)}, {x1, y0, dx * (1 - dy)}, {x0, y1, (1 - dx) * dy}, {x1, y1, dx * dy}} {
				c := color.NRGBAModel.Convert(source.At(bounds.Min.X+point.x, bounds.Min.Y+point.y)).(color.NRGBA)
				for i, v := range []uint8{c.R, c.G, c.B, c.A} {
					sums[i] += float64(v) * point.weight
				}
			}
			output.SetNRGBA(x, y, color.NRGBA{uint8(sums[0] + 0.5), uint8(sums[1] + 0.5), uint8(sums[2] + 0.5), uint8(sums[3] + 0.5)})
		}
	}
	return output
}

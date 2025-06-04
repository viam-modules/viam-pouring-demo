package pour

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
)

const pick_expectedX = 1280
const pick_expectedY = 720
const pick_xSize = 650
const pick_ySize = 250
const pick_xOffset = 550
const pick_yOffset = pick_expectedY - pick_ySize

var pick_x = 1

func writeImage(img image.Image, fn string) error {
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, img, nil)
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func checkPick(img image.Image) (bool, error) {
	pick_x++

	edges, err := prepPickImage(img)
	if err != nil {
		return false, err
	}

	err = writeImage(edges, fmt.Sprintf("temp-edges-%d.jpg", pick_x))
	if err != nil {
		return false, err
	}

	return false, nil
}

func prepPickImage(img image.Image) (*image.Gray, error) {
	bounds := img.Bounds()
	if bounds.Dx() != pick_expectedX || bounds.Dy() != pick_expectedY {
		return nil, fmt.Errorf("invalid dimensions: %v x %v", bounds.Dx(), bounds.Dy())
	}

	gray := pickConvertToGrayscale(img)
	edges := detectEdges(gray)

	return edges, nil
}

func pickConvertToGrayscale(img image.Image) *image.Gray {
	bounds := image.Rect(0, 0, pick_xSize, pick_ySize)
	gray := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			originalColor := img.At(x+pick_xOffset, y+pick_yOffset)
			grayColor := color.GrayModel.Convert(originalColor)
			gray.Set(x, y, grayColor)
		}
	}
	return gray
}

func detectEdges(img *image.Gray) *image.Gray {
	bounds := img.Bounds()
	edges := image.NewGray(bounds)

	sobelX := [][]int{
		{-1, 0, 1},
		{-2, 0, 2},
		{-1, 0, 1},
	}

	sobelY := [][]int{
		{-1, -2, -1},
		{0, 0, 0},
		{1, 2, 1},
	}

	for y := 1; y < bounds.Max.Y-1; y++ {
		for x := 1; x < bounds.Max.X-1; x++ {
			var gx, gy int

			for i := -1; i <= 1; i++ {
				for j := -1; j <= 1; j++ {
					pixel := int(img.GrayAt(x+j, y+i).Y)
					gx += pixel * sobelX[i+1][j+1]
					gy += pixel * sobelY[i+1][j+1]
				}
			}

			magnitude := int(math.Sqrt(float64(gx*gx + gy*gy)))
			if magnitude > 255 {
				magnitude = 255
			}
			edges.SetGray(x, y, color.Gray{uint8(magnitude)})
		}
	}
	return edges
}

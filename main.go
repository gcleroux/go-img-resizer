package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/disintegration/imaging"
	"github.com/jung-kurt/gofpdf"
)

//go:embed templates/index.html
//go:embed static/*
var content embed.FS

func main() {
	port := flag.Int("port", 8080, "Port to serve on")
	addr := flag.String("addr", "localhost", "Address to bind to")
	noBrowser := flag.Bool("no-browser", false, "Do not open the browser automatically")
	flag.Parse()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/generate", generatePDFHandler)

	// Serve static files (CSS, JS) from the embedded filesystem.
	staticFiles, err := fs.Sub(content, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))

	url := fmt.Sprintf("http://%s:%d", *addr, *port)

	if !*noBrowser {
		go openBrowser(url)
	}

	log.Printf("Starting server at %s\n", url)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *addr, *port), nil); err != nil {
		log.Fatal(err)
	}
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	default:
		log.Printf("Please open your browser and navigate to: %s\n", url)
	}

	if err != nil {
		log.Printf("Failed to open browser: %v\n", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	// Load the template from the embedded filesystem.
	tmpl, err := template.ParseFS(content, "templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func generatePDFHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form with a 20 MB limit.
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get frame dimensions in inches.
	frameWidthStr := r.FormValue("frameWidth")
	frameHeightStr := r.FormValue("frameHeight")
	frameWidth, err := strconv.ParseFloat(frameWidthStr, 64)
	if err != nil || frameWidth <= 0 {
		frameWidth = 8.0
	}
	frameHeight, err := strconv.ParseFloat(frameHeightStr, 64)
	if err != nil || frameHeight <= 0 {
		frameHeight = 10.0
	}

	// Get DPI.
	dpiStr := r.FormValue("dpi")
	dpi, err := strconv.Atoi(dpiStr)
	if err != nil || dpi <= 0 {
		dpi = 300
	}

	// Get rotation option.
	rotateOption := r.FormValue("rotate")
	rotate := (rotateOption == "on")

	// Get keep aspect option.
	keepAspectOption := r.FormValue("keepAspect")
	keepAspect := (keepAspectOption == "on")

	// Get crop option.
	cropOption := r.FormValue("crop")
	crop := (cropOption == "on")

	// If rotated, swap the effective frame dimensions.
	effFrameWidth := frameWidth
	effFrameHeight := frameHeight

	// Convert effective frame dimensions to millimeters for PDF.
	frameWidthMM := effFrameWidth * 25.4
	frameHeightMM := effFrameHeight * 25.4

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(5, 10, 5)
	pdf.SetAutoPageBreak(true, 10)
	pdf.SetLineWidth(0.5) // thin border

	// Retrieve the files from form data (submitted under key "images").
	files := r.MultipartForm.File["images"]

	// Process each uploaded image.
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			continue
		}

		// Read file into a buffer.
		var fileBuf bytes.Buffer
		if _, err := io.Copy(&fileBuf, file); err != nil {
			file.Close()
			continue
		}
		file.Close()

		// Decode the image.
		srcImg, _, err := image.Decode(bytes.NewReader(fileBuf.Bytes()))
		if err != nil {
			continue
		}

		// Rotate if requested.
		if rotate {
			srcImg = imaging.Rotate90(srcImg)
		}

		// Compute target pixel dimensions.
		targetWidthPx := int(effFrameWidth * float64(dpi))
		targetHeightPx := int(effFrameHeight * float64(dpi))

		var resizedImg *image.NRGBA
		if crop {
			// Crop and scale the image to fill the frame.
			resizedImg = imaging.Fill(srcImg, targetWidthPx, targetHeightPx, imaging.Center, imaging.Lanczos)
		} else if keepAspect {
			// Resize proportionally so the whole image fits.
			resizedImg = imaging.Fit(srcImg, targetWidthPx, targetHeightPx, imaging.Lanczos)
		} else {
			// Force a resize to exactly target dimensions.
			resizedImg = imaging.Resize(srcImg, targetWidthPx, targetHeightPx, imaging.Lanczos)
		}
		// Prepare for PDF placement.
		var posX, posY float64
		if keepAspect {
			// Compute actual printed size in mm from the resized image.
			actualWidthPx := resizedImg.Bounds().Dx()
			actualHeightPx := resizedImg.Bounds().Dy()
			printedWidthMM := float64(actualWidthPx) / float64(dpi) * 25.4
			printedHeightMM := float64(actualHeightPx) / float64(dpi) * 25.4
			// Center the image within the frame (with a 10mm margin offset).
			posX = 5 + (frameWidthMM-printedWidthMM)/2
			posY = 5 + (frameHeightMM-printedHeightMM)/2
		} else {
			// If forced resize, the image fills the frame.
			posX, posY = 5, 5
		}

		// Encode the processed image to PNG.
		var imgBuf bytes.Buffer
		if err := png.Encode(&imgBuf, resizedImg); err != nil {
			continue
		}

		// Add a new PDF page.
		pdf.AddPage()

		// Register the image with PDF.
		options := gofpdf.ImageOptions{
			ImageType: "PNG",
			ReadDpi:   false,
		}
		imgName := fileHeader.Filename
		pdf.RegisterImageOptionsReader(imgName, options, &imgBuf)

		// Place the image on the page.
		pdf.ImageOptions(imgName, posX, posY, printedOrForcedWidth(keepAspect, &resizedImg, dpi, frameWidthMM), printedOrForcedHeight(keepAspect, &resizedImg, dpi, frameHeightMM), false, options, 0, "")

		// Draw the thin black border representing the full frame.
		pdf.Rect(5, 5, frameWidthMM, frameHeightMM, "D")
	}

	var pdfBuf bytes.Buffer
	if err := pdf.Output(&pdfBuf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=output.pdf")
	w.Write(pdfBuf.Bytes())
}

// Helper functions to return printed dimensions for the image placement.
// If keepAspect is false, the image fills the frame exactly.
// If true, we compute from the resized image's actual pixel dimensions.
func printedOrForcedWidth(keepAspect bool, img *(*image.NRGBA), dpi int, frameWidthMM float64) float64 {
	if !keepAspect {
		return frameWidthMM
	}
	actualWidthPx := (*img).Bounds().Dx()
	return float64(actualWidthPx) / float64(dpi) * 25.4
}

func printedOrForcedHeight(keepAspect bool, img *(*image.NRGBA), dpi int, frameHeightMM float64) float64 {
	if !keepAspect {
		return frameHeightMM
	}
	actualHeightPx := (*img).Bounds().Dy()
	return float64(actualHeightPx) / float64(dpi) * 25.4
}

package main

import (
	"syscall/js"
	"strings"
)

var (
	canvas       js.Value
	ctx          js.Value
	canvasWidth  int
	canvasHeight int
	elements     []Element
	scrollY      float64
	zoomLevel    float64 = 1.0
	contentHeight int
	loadedImages map[string]js.Value
	markdownText string
	dpr          float64
)

type ElementType int

const (
	TypeText ElementType = iota
	TypeImage
	TypeDivider
)

type Element struct {
	elemType  ElementType
	text      string
	fontSize  int
	y         int
	x         int
	color     string
	isHeading bool
	headingLevel int
	isBold    bool
	isItalic  bool
	isCode    bool
	imageURL  string
	imageObj  js.Value
	imgWidth  int
	imgHeight int
}

func main() {
	document := js.Global().Get("document")
	body := document.Get("body")
	window := js.Global().Get("window")

	loadedImages = make(map[string]js.Value)

	canvas = document.Call("createElement", "canvas")
	body.Call("appendChild", canvas)

	ctx = canvas.Call("getContext", "2d")

	style := document.Call("createElement", "style")
	style.Set("textContent", `
		* {
			margin: 0;
			padding: 0;
			box-sizing: border-box;
		}
		body {
			overflow: hidden;
			background: #fafafa;
		}
		canvas {
			display: block;
			image-rendering: -webkit-optimize-contrast;
			image-rendering: crisp-edges;
		}
	`)
	document.Get("head").Call("appendChild", style)

	promise := js.Global().Call("fetch", "main.md")
	
	then1 := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		response := args[0]
		return response.Call("text")
	})
	defer then1.Release()

	then2 := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		text := args[0].String()
		markdownText = text
		
		// Get device pixel ratio for sharp rendering
		dpr = window.Get("devicePixelRatio").Float()
		if dpr < 1 {
			dpr = 1
		}
		
		resizeCanvas := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			width := window.Get("innerWidth").Int()
			height := window.Get("innerHeight").Int()
			
			// Set display size (CSS pixels)
			canvas.Get("style").Set("width", intToString(width)+"px")
			canvas.Get("style").Set("height", intToString(height)+"px")
			
			// Set actual size in memory (scaled for DPI)
			canvas.Set("width", int(float64(width)*dpr))
			canvas.Set("height", int(float64(height)*dpr))
			
			// Scale context to match
			ctx.Call("scale", dpr, dpr)
			
			// Store logical dimensions
			canvasWidth = width
			canvasHeight = height
			
			// Enable sharp text rendering
			ctx.Set("imageSmoothingEnabled", false)
			ctx.Set("fontSmooth", "never")
			ctx.Set("textRendering", "geometricPrecision")
			
			parseMarkdown(markdownText)
			render()
			return nil
		})
		resizeCanvas.Invoke()
		resizeCanvas.Release()

		window.Call("addEventListener", "resize", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			width := window.Get("innerWidth").Int()
			height := window.Get("innerHeight").Int()
			
			// Set display size (CSS pixels)
			canvas.Get("style").Set("width", intToString(width)+"px")
			canvas.Get("style").Set("height", intToString(height)+"px")
			
			// Set actual size in memory (scaled for DPI)
			canvas.Set("width", int(float64(width)*dpr))
			canvas.Set("height", int(float64(height)*dpr))
			
			// Scale context to match
			ctx.Call("scale", dpr, dpr)
			
			// Store logical dimensions
			canvasWidth = width
			canvasHeight = height
			
			// Enable sharp text rendering
			ctx.Set("imageSmoothingEnabled", false)
			
			parseMarkdown(markdownText)
			render()
			return nil
		}))

		canvas.Call("addEventListener", "wheel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			event := args[0]
			event.Call("preventDefault")
			
			if event.Get("ctrlKey").Bool() {
				deltaY := event.Get("deltaY").Float()
				zoomLevel -= deltaY * 0.001
				
				if zoomLevel < 0.7 {
					zoomLevel = 0.7
				}
				if zoomLevel > 2.0 {
					zoomLevel = 2.0
				}
				
				parseMarkdown(markdownText)
				render()
			} else {
				deltaY := event.Get("deltaY").Float()
				scrollY += deltaY
				
				maxScroll := float64(contentHeight - canvasHeight)
				if maxScroll < 0 {
					maxScroll = 0
				}
				if scrollY < 0 {
					scrollY = 0
				}
				if scrollY > maxScroll {
					scrollY = maxScroll
				}
				
				render()
			}
			
			return nil
		}), map[string]interface{}{"passive": false})
		
		return nil
	})
	defer then2.Release()

	promise.Call("then", then1).Call("then", then2)

	select {}
}

func parseMarkdown(content string) {
	elements = []Element{}
	rawLines := strings.Split(content, "\n")
	
	baseSize := int(18.0 * zoomLevel)
	currentY := int(80.0 * zoomLevel)
	
	contentWidth := 700
	if canvasWidth < 900 {
		contentWidth = canvasWidth - 80
	}
	contentWidth = int(float64(contentWidth) * zoomLevel)
	
	margin := (canvasWidth - contentWidth) / 2
	if margin < int(40.0*zoomLevel) {
		margin = int(40.0 * zoomLevel)
		contentWidth = canvasWidth - margin*2
	}

	inCodeBlock := false
	codeBlockLines := []string{}

	for i := 0; i < len(rawLines); i++ {
		line := rawLines[i]
		trimmed := strings.TrimSpace(line)
		
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				// End code block
				currentY += renderCodeBlock(codeBlockLines, margin, currentY, contentWidth, baseSize)
				codeBlockLines = []string{}
				inCodeBlock = false
				currentY += int(30.0 * zoomLevel)
			} else {
				// Start code block
				inCodeBlock = true
				currentY += int(20.0 * zoomLevel)
			}
			continue
		}
		
		if inCodeBlock {
			codeBlockLines = append(codeBlockLines, line)
			continue
		}
		
		if trimmed == "" {
			currentY += int(20.0 * zoomLevel)
			continue
		}

		// Horizontal rules
		if trimmed == "---" || trimmed == "***" {
			elements = append(elements, Element{
				elemType: TypeDivider,
				y:        currentY,
				x:        margin,
			})
			currentY += int(40.0 * zoomLevel)
			continue
		}

		// Images
		if strings.HasPrefix(trimmed, "![") && strings.Contains(trimmed, "](") {
			endAlt := strings.Index(trimmed, "]")
			startURL := strings.Index(trimmed, "](")
			endURL := strings.Index(trimmed[startURL:], ")")
			if endAlt > 0 && startURL > 0 && endURL > 0 {
				imageURL := trimmed[startURL+2 : startURL+endURL]
				currentY += loadAndAddImage(imageURL, margin, currentY, contentWidth)
				continue
			}
		}

		fontSize := baseSize
		text := trimmed
		color := "#2c3e50"
		isHeading := false
		headingLevel := 0

		// Headers
		if strings.HasPrefix(trimmed, "# ") {
			fontSize = int(42.0 * zoomLevel)
			text = strings.TrimPrefix(trimmed, "# ")
			color = "#1a1a1a"
			isHeading = true
			headingLevel = 1
			currentY += int(20.0 * zoomLevel)
		} else if strings.HasPrefix(trimmed, "## ") {
			fontSize = int(32.0 * zoomLevel)
			text = strings.TrimPrefix(trimmed, "## ")
			color = "#1a1a1a"
			isHeading = true
			headingLevel = 2
			currentY += int(30.0 * zoomLevel)
		} else if strings.HasPrefix(trimmed, "### ") {
			fontSize = int(24.0 * zoomLevel)
			text = strings.TrimPrefix(trimmed, "### ")
			color = "#2c3e50"
			isHeading = true
			headingLevel = 3
			currentY += int(20.0 * zoomLevel)
		}

		// Lists
		indent := 0
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			text = strings.TrimPrefix(text, "- ")
			text = strings.TrimPrefix(text, "* ")
			text = "• " + text
			indent = int(20.0 * zoomLevel)
		}

		// Inline formatting
		isBold := false
		isItalic := false
		isCode := false
		
		if strings.Contains(text, "**") {
			text = strings.ReplaceAll(text, "**", "")
			isBold = true
		}
		if strings.Contains(text, "*") && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") {
			text = strings.ReplaceAll(text, "*", "")
			isItalic = true
		}
		if strings.Contains(text, "`") {
			text = strings.ReplaceAll(text, "`", "")
			isCode = true
			color = "#c7254e"
		}

		currentY += addWrappedText(text, fontSize, margin+indent, currentY, contentWidth-indent, color, isBold, isItalic, isCode, isHeading, headingLevel)
		
		if isHeading {
			currentY += int(15.0 * zoomLevel)
		} else {
			currentY += int(8.0 * zoomLevel)
		}
	}
	
	contentHeight = currentY + int(100.0*zoomLevel)
}

func renderCodeBlock(lines []string, x, y, width, baseSize int) int {
	if len(lines) == 0 {
		return 0
	}
	
	bgHeight := len(lines)*int(28.0*zoomLevel) + int(30.0*zoomLevel)
	
	// Background
	elements = append(elements, Element{
		elemType: TypeText,
		text:     "", // Special marker for code block background
		y:        y,
		x:        x,
		fontSize: bgHeight,
	})
	
	currentY := y + int(15.0*zoomLevel)
	fontSize := int(15.0 * zoomLevel)
	
	for _, line := range lines {
		elements = append(elements, Element{
			elemType: TypeText,
			text:     line,
			fontSize: fontSize,
			y:        currentY,
			x:        x + int(15.0*zoomLevel),
			color:    "#333",
			isCode:   true,
		})
		currentY += int(28.0 * zoomLevel)
	}
	
	return bgHeight
}

func loadAndAddImage(url string, x, y, maxWidth int) int {
	if img, exists := loadedImages[url]; exists {
		if img.Get("complete").Bool() {
			imgWidth := img.Get("naturalWidth").Int()
			imgHeight := img.Get("naturalHeight").Int()
			
			if imgWidth == 0 || imgHeight == 0 {
				imgWidth = 800
				imgHeight = 400
			}
			
			scale := float64(maxWidth) / float64(imgWidth)
			if scale > 1.0 {
				scale = 1.0
			}
			
			scaledWidth := int(float64(imgWidth) * scale)
			scaledHeight := int(float64(imgHeight) * scale)
			
			elements = append(elements, Element{
				elemType:  TypeImage,
				y:         y + int(20.0*zoomLevel),
				x:         x,
				imageURL:  url,
				imageObj:  img,
				imgWidth:  scaledWidth,
				imgHeight: scaledHeight,
			})
			
			return scaledHeight + int(60.0*zoomLevel)
		}
	}
	
	document := js.Global().Get("document")
	img := document.Call("createElement", "img")
	img.Set("crossOrigin", "anonymous")
	img.Set("src", url)
	
	loadedImages[url] = img
	
	elements = append(elements, Element{
		elemType:  TypeImage,
		y:         y + int(20.0*zoomLevel),
		x:         x,
		imageURL:  url,
		imageObj:  img,
		imgWidth:  maxWidth,
		imgHeight: int(float64(maxWidth) / 2.0),
	})
	
	img.Set("onload", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		parseMarkdown(markdownText)
		render()
		return nil
	}))
	
	return int(float64(maxWidth)/2.0) + int(60.0*zoomLevel)
}

func addWrappedText(text string, fontSize, x, y, maxWidth int, color string, isBold, isItalic, isCode, isHeading bool, headingLevel int) int {
	if maxWidth <= 0 {
		return int(float64(fontSize) * 1.5)
	}
	
	font := ""
	if isHeading {
		font = "700 "
	} else if isBold {
		font = "600 "
	} else {
		font = "400 "
	}
	
	if isCode {
		font += intToString(fontSize) + "px 'Courier New', monospace"
	} else {
		font += intToString(fontSize) + "px -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
	}
	
	ctx.Set("font", font)
	
	words := strings.Fields(text)
	if len(words) == 0 {
		return int(float64(fontSize) * 1.5)
	}
	
	currentLine := ""
	lineHeight := int(float64(fontSize) * 1.6)
	totalHeight := 0
	
	for _, word := range words {
		testLine := currentLine
		if currentLine != "" {
			testLine += " "
		}
		testLine += word
		
		metrics := ctx.Call("measureText", testLine)
		width := metrics.Get("width").Float()
		
		if width > float64(maxWidth) && currentLine != "" {
			elements = append(elements, Element{
				elemType: TypeText,
				text:     currentLine,
				fontSize: fontSize,
				y:        y + totalHeight,
				x:        x,
				color:    color,
				isBold:   isBold,
				isItalic: isItalic,
				isCode:   isCode,
				isHeading: isHeading,
				headingLevel: headingLevel,
			})
			totalHeight += lineHeight
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	
	if currentLine != "" {
		elements = append(elements, Element{
			elemType: TypeText,
			text:     currentLine,
			fontSize: fontSize,
			y:        y + totalHeight,
			x:        x,
			color:    color,
			isBold:   isBold,
			isItalic: isItalic,
			isCode:   isCode,
			isHeading: isHeading,
			headingLevel: headingLevel,
		})
		totalHeight += lineHeight
	}
	
	return totalHeight
}

func render() {
	// Disable smoothing for sharp rendering
	ctx.Set("imageSmoothingEnabled", false)
	
	// Background
	ctx.Set("fillStyle", "#fafafa")
	ctx.Call("fillRect", 0, 0, canvasWidth, canvasHeight)
	
	// Re-enable font smoothing settings
	ctx.Set("textRendering", "optimizeLegibility")
	
	for _, elem := range elements {
		adjustedY := elem.y - int(scrollY)
		
		if adjustedY > -300 && adjustedY < canvasHeight+300 {
			switch elem.elemType {
			case TypeImage:
				drawImage(elem, adjustedY)
			case TypeDivider:
				drawDivider(elem, adjustedY)
			case TypeText:
				if elem.text == "" && elem.fontSize > 0 {
					// Code block background
					drawCodeBlockBg(elem, adjustedY)
				} else {
					drawText(elem, adjustedY)
				}
			}
		}
	}
}

func drawImage(elem Element, y int) {
	if !elem.imageObj.Get("complete").Bool() {
		return
	}
	
	// Temporarily enable smoothing for images
	ctx.Set("imageSmoothingEnabled", true)
	ctx.Set("imageSmoothingQuality", "high")
	
	ctx.Set("shadowColor", "rgba(0, 0, 0, 0.1)")
	ctx.Set("shadowBlur", 20)
	ctx.Set("shadowOffsetY", 4)
	ctx.Call("drawImage", elem.imageObj, elem.x, y, elem.imgWidth, elem.imgHeight)
	ctx.Set("shadowBlur", 0)
	ctx.Set("shadowOffsetY", 0)
	
	// Disable smoothing again for text
	ctx.Set("imageSmoothingEnabled", false)
}

func drawDivider(elem Element, y int) {
	contentWidth := 700
	if canvasWidth < 900 {
		contentWidth = canvasWidth - 80
	}
	contentWidth = int(float64(contentWidth) * zoomLevel)
	
	ctx.Set("strokeStyle", "#e0e0e0")
	ctx.Set("lineWidth", 1)
	ctx.Call("beginPath")
	ctx.Call("moveTo", elem.x, y)
	ctx.Call("lineTo", elem.x+contentWidth, y)
	ctx.Call("stroke")
}

func drawCodeBlockBg(elem Element, y int) {
	contentWidth := 700
	if canvasWidth < 900 {
		contentWidth = canvasWidth - 80
	}
	contentWidth = int(float64(contentWidth) * zoomLevel)
	
	ctx.Set("fillStyle", "#f5f5f5")
	ctx.Call("fillRect", elem.x, y, contentWidth, elem.fontSize)
	
	ctx.Set("strokeStyle", "#e0e0e0")
	ctx.Set("lineWidth", 1)
	ctx.Call("strokeRect", elem.x, y, contentWidth, elem.fontSize)
}

func drawText(elem Element, y int) {
	font := ""
	if elem.isHeading {
		font = "700 "
	} else if elem.isBold {
		font = "600 "
	} else {
		font = "400 "
	}
	
	if elem.isCode {
		font += intToString(elem.fontSize) + "px 'Courier New', monospace"
	} else {
		font += intToString(elem.fontSize) + "px -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
	}
	
	ctx.Set("font", font)
	ctx.Set("fillStyle", elem.color)
	ctx.Set("textAlign", "left")
	ctx.Set("textBaseline", "top")
	
	// Round to nearest pixel for sharpest rendering
	x := float64(elem.x)
	yPos := float64(y)
	
	// Enable subpixel antialiasing
	ctx.Set("textRendering", "optimizeLegibility")
	
	ctx.Call("fillText", elem.text, x, yPos)
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		result = string(rune('0'+(i%10))) + result
		i /= 10
	}
	if neg {
		result = "-" + result
	}
	return result
}


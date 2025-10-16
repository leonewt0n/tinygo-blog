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
	lines        []TextLine
	scrollY      float64
	zoomLevel    float64 = 1.0
	contentHeight int
)

type TextLine struct {
	text     string
	fontSize int
	y        int
}

func main() {
	document := js.Global().Get("document")
	body := document.Get("body")
	window := js.Global().Get("window")

	// Create canvas
	canvas = document.Call("createElement", "canvas")
	body.Call("appendChild", canvas)

	// Get 2D context
	ctx = canvas.Call("getContext", "2d")

	// Add styling for fullscreen canvas
	style := document.Call("createElement", "style")
	style.Set("textContent", `
		* {
			margin: 0;
			padding: 0;
			box-sizing: border-box;
		}
		body {
			overflow: hidden;
			background: #fff;
		}
		canvas {
			display: block;
			width: 100vw;
			height: 100vh;
			cursor: default;
		}
	`)
	document.Get("head").Call("appendChild", style)

	// Fetch main.md
	promise := js.Global().Call("fetch", "main.md")
	
	then1 := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		response := args[0]
		return response.Call("text")
	})
	defer then1.Release()

	then2 := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		text := args[0].String()
		
		// Initial resize
		resizeCanvas := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			width := window.Get("innerWidth").Int()
			height := window.Get("innerHeight").Int()
			canvas.Set("width", width)
			canvas.Set("height", height)
			canvasWidth = width
			canvasHeight = height
			parseMarkdown(text)
			return nil
		})
		resizeCanvas.Invoke()
		resizeCanvas.Release()

		// Add resize listener
		window.Call("addEventListener", "resize", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			width := window.Get("innerWidth").Int()
			height := window.Get("innerHeight").Int()
			canvas.Set("width", width)
			canvas.Set("height", height)
			canvasWidth = width
			canvasHeight = height
			parseMarkdown(text)
			return nil
		}))

		// Add wheel listener for scroll and pinch-to-zoom
		canvas.Call("addEventListener", "wheel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			event := args[0]
			event.Call("preventDefault")
			
			// Check if it's a pinch gesture (ctrl+wheel or pinch on trackpad)
			if event.Get("ctrlKey").Bool() {
				// Pinch to zoom
				deltaY := event.Get("deltaY").Float()
				zoomLevel -= deltaY * 0.001
				
				// Clamp zoom level
				if zoomLevel < 0.5 {
					zoomLevel = 0.5
				}
				if zoomLevel > 3.0 {
					zoomLevel = 3.0
				}
				
				parseMarkdown(text)
			} else {
				// Regular scroll
				deltaY := event.Get("deltaY").Float()
				scrollY += deltaY
				
				// Clamp scroll
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
			}
			
			return nil
		}), map[string]interface{}{"passive": false})

		// Start animation
		var animate js.Func
		animate = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			render()
			js.Global().Call("requestAnimationFrame", animate)
			return nil
		})
		js.Global().Call("requestAnimationFrame", animate)
		
		return nil
	})
	defer then2.Release()

	promise.Call("then", then1).Call("then", then2)

	// Keep program running
	select {}
}

func parseMarkdown(content string) {
	lines = []TextLine{}
	rawLines := strings.Split(content, "\n")
	
	baseSize := int(float64(40) * zoomLevel)
	currentY := int(float64(80) * zoomLevel)
	lineHeight := int(float64(60) * zoomLevel)
	margin := int(float64(40) * zoomLevel)
	maxWidth := canvasWidth - (margin * 2)

	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			currentY += int(float64(30) * zoomLevel)
			continue
		}

		fontSize := baseSize
		text := line

		// Parse headers
		if strings.HasPrefix(line, "# ") {
			fontSize = int(float64(baseSize) * 2.0)
			text = strings.TrimPrefix(line, "# ")
			lineHeight = int(float64(90) * zoomLevel)
		} else if strings.HasPrefix(line, "## ") {
			fontSize = int(float64(baseSize) * 1.5)
			text = strings.TrimPrefix(line, "## ")
			lineHeight = int(float64(75) * zoomLevel)
		} else if strings.HasPrefix(line, "### ") {
			fontSize = int(float64(baseSize) * 1.2)
			text = strings.TrimPrefix(line, "### ")
			lineHeight = int(float64(65) * zoomLevel)
		} else {
			lineHeight = int(float64(60) * zoomLevel)
		}

		// Remove markdown formatting
		text = strings.ReplaceAll(text, "**", "")
		text = strings.ReplaceAll(text, "*", "")
		text = strings.ReplaceAll(text, "`", "")

		// Word wrap
		wrappedLines := wrapText(text, fontSize, maxWidth)
		for _, wrappedLine := range wrappedLines {
			lines = append(lines, TextLine{
				text:     wrappedLine,
				fontSize: fontSize,
				y:        currentY,
			})
			currentY += lineHeight
		}
	}
	
	contentHeight = currentY + int(float64(80)*zoomLevel)
}

func wrapText(text string, fontSize int, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	
	// Set font for measurement
	ctx.Set("font", "bold "+intToString(fontSize)+"px Arial")
	
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	
	var lines []string
	currentLine := ""
	
	for _, word := range words {
		testLine := currentLine
		if currentLine != "" {
			testLine += " "
		}
		testLine += word
		
		metrics := ctx.Call("measureText", testLine)
		width := metrics.Get("width").Float()
		
		if width > float64(maxWidth) && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	if len(lines) == 0 {
		return []string{text}
	}
	
	return lines
}

func render() {
	ctx.Call("clearRect", 0, 0, canvasWidth, canvasHeight)

	margin := int(float64(40) * zoomLevel)
	
	for _, line := range lines {
		// Only render lines that are visible
		adjustedY := line.y - int(scrollY)
		if adjustedY > -100 && adjustedY < canvasHeight+100 {
			drawGlassText(line.text, margin, adjustedY, line.fontSize)
		}
	}
}

func drawGlassText(text string, x, y, fontSize int) {
	ctx.Set("font", "bold "+intToString(fontSize)+"px Arial")
	ctx.Set("textAlign", "left")
	ctx.Set("textBaseline", "top")

	// Layer 1: Shadow
	ctx.Set("shadowBlur", 0)
	ctx.Set("shadowColor", "transparent")
	ctx.Set("fillStyle", "rgba(100, 150, 200, 0.1)")
	ctx.Call("fillText", text, x+4, y+4)

	// Layer 2: Dark outline
	ctx.Set("fillStyle", "rgba(80, 140, 200, 0.3)")
	ctx.Call("fillText", text, x+2, y+2)

	// Layer 3: Main glass body
	ctx.Set("fillStyle", "rgba(150, 200, 255, 0.4)")
	ctx.Call("fillText", text, x, y)

	// Layer 4: Lighter middle
	ctx.Set("fillStyle", "rgba(180, 220, 255, 0.5)")
	ctx.Call("fillText", text, x-1, y-1)

	// Layer 5: Bright highlight
	ctx.Set("fillStyle", "rgba(220, 240, 255, 0.7)")
	ctx.Call("fillText", text, x-2, y-3)

	// Layer 6: Sharp white highlight
	ctx.Set("fillStyle", "rgba(255, 255, 255, 0.6)")
	ctx.Set("shadowBlur", float64(fontSize)*0.15)
	ctx.Set("shadowColor", "rgba(200, 230, 255, 0.8)")
	ctx.Call("fillText", text, x-3, y-4)

	// Layer 7: Subtle glow
	ctx.Set("shadowBlur", float64(fontSize)*0.3)
	ctx.Set("shadowColor", "rgba(150, 200, 255, 0.3)")
	ctx.Set("fillStyle", "rgba(180, 220, 255, 0.2)")
	ctx.Call("fillText", text, x, y)
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+(i%10))) + result
		i /= 10
	}
	return result
}


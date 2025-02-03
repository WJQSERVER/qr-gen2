package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"image/color"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
)

var (
	//go:embed static/*
	staticFiles embed.FS
	staticCache = make(map[string][]byte) // 静态文件缓存
)

func init() {
	// 预加载静态文件到内存
	if err := fs.WalkDir(staticFiles, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := staticFiles.ReadFile(path)
		if err != nil {
			return err
		}
		key := strings.TrimPrefix(path, "static/")
		staticCache[key] = data
		return nil
	}); err != nil {
		log.Fatal("Failed to initialize static cache:", err)
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode) // 生产模式
	r := gin.Default()

	// API路由
	r.GET("/api/generate", generateQRHandler)

	// 静态文件路由
	r.GET("/", func(c *gin.Context) {
		if data, ok := staticCache["index.html"]; ok {
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
		} else {
			c.String(http.StatusInternalServerError, "Failed to load index.html")
		}
	})

	r.GET("/static/:file", func(c *gin.Context) {
		fileName := c.Param("file")
		if data, ok := staticCache[fileName]; ok {
			contentType := getContentType(fileName)
			c.Data(http.StatusOK, contentType, data)
		} else {
			c.String(http.StatusNotFound, "File not found")
		}
	})

	if err := r.Run(":8080"); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func generateQRHandler(c *gin.Context) {
	// 参数解析
	url := c.Query("url")
	level := c.DefaultQuery("level", "L")
	sizeStr := c.DefaultQuery("size", "256")
	colorStr := c.DefaultQuery("color", "000000")

	// 参数校验
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url parameter is required"})
		return
	}

	// 解码base64 url
	url, err := decodeBase64URL(url)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url parameter"})
		return
	}

	// 尺寸参数处理
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size <= 0 || size > 2048 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid size parameter (max 2048)"})
		return
	}

	// 颜色处理
	r, g, b, err := hexToRGB(colorStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 容错等级
	qrLevel := qrcode.Low
	switch strings.ToUpper(level) {
	case "M":
		qrLevel = qrcode.Medium
	case "Q":
		qrLevel = qrcode.High
	case "H":
		qrLevel = qrcode.Highest
	}

	// 生成二维码
	qr, err := qrcode.New(url, qrLevel)
	if err != nil {
		log.Printf("QR generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate QR code"})
		return
	}

	qr.BackgroundColor = color.White
	qr.ForegroundColor = color.RGBA{uint8(r), uint8(g), uint8(b), 255}

	// 编码为PNG
	var pngBuffer bytes.Buffer
	if err := qr.Write(size, &pngBuffer); err != nil {
		log.Printf("QR encoding failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode QR code"})
		return
	}

	c.Data(http.StatusOK, "image/png", pngBuffer.Bytes())
}

func hexToRGB(hex string) (int, int, int, error) {
	hex = strings.TrimPrefix(hex, "#")
	switch len(hex) {
	case 3:
		hex = fmt.Sprintf("%c%c%c%c%c%c", hex[0], hex[0], hex[1], hex[1], hex[2], hex[2])
	case 6:
	default:
		return 0, 0, 0, fmt.Errorf("invalid color format, expected 3 or 6 characters")
	}

	r, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid red component")
	}

	g, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid green component")
	}

	b, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid blue component")
	}

	return int(r), int(g), int(b), nil
}

func getContentType(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(filename, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(filename, ".png"):
		return "image/png"
	case strings.HasSuffix(filename, ".jpg"), strings.HasSuffix(filename, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(filename, ".html"):
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// 解码base64 url
func decodeBase64URL(s string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

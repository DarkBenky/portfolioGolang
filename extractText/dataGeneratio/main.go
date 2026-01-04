package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	SamplesPerFile = 256
	GENERIC_TEXTS  = iota
	MATH_TEXTS
	CODE_MIX_TEXTS
	OPEN_THOUGHTS_TEXTS
	FINE_BERT_TEXTS
	SCIENCE_TEXTS // ArXiv and other scientific texts
	PATH_TO_SAVE  = "/media/user/free/data/data"
)

type TokenizedTexts struct {
	Text          string `json:"text"`
	TokenizedText []int  `json:"tokenized_text"`
	Category      int    `json:"category"`
}

type SaveDataSample struct {
	Category       uint8
	NumberOfTokens uint64
	TokenizedText  []uint32
}

type SaveFile struct {
	Samples [SamplesPerFile]SaveDataSample
}

func writeSaveFileToDisc(saveFile *SaveFile, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	for i := 0; i < SamplesPerFile; i++ {
		sample := &saveFile.Samples[i]

		if err := binary.Write(file, binary.LittleEndian, sample.Category); err != nil {
			return err
		}

		if err := binary.Write(file, binary.LittleEndian, sample.NumberOfTokens); err != nil {
			return err
		}

		for _, token := range sample.TokenizedText {
			if err := binary.Write(file, binary.LittleEndian, token); err != nil {
				return err
			}
		}
	}

	return nil
}

func readSaveFileFromDisc(filePath string) (*SaveFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var saveFile SaveFile

	for i := 0; i < SamplesPerFile; i++ {
		sample := &saveFile.Samples[i]

		if err := binary.Read(file, binary.LittleEndian, &sample.Category); err != nil {
			return nil, err
		}

		if err := binary.Read(file, binary.LittleEndian, &sample.NumberOfTokens); err != nil {
			return nil, err
		}

		sample.TokenizedText = make([]uint32, sample.NumberOfTokens)
		for j := uint64(0); j < sample.NumberOfTokens; j++ {
			if err := binary.Read(file, binary.LittleEndian, &sample.TokenizedText[j]); err != nil {
				return nil, err
			}
		}
	}

	return &saveFile, nil
}

func getRandomSaveFile() (*SaveFile, error) {
	files, err := filepath.Glob(filepath.Join(PATH_TO_SAVE, "*.bin"))
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no save files found")
	}

	rand.Seed(time.Now().UnixNano())
	randomFile := files[rand.Intn(len(files))]

	return readSaveFileFromDisc(randomFile)
}

var (
	ProcessedTokens uint64 = 0
	mu              sync.Mutex
	currentFile     SaveFile
	currentIndex    int
	fileCounter     int
	requestCount    uint64
	startTime       time.Time
)

func init() {
	startTime = time.Now()
}

func saveData(c echo.Context) error {
	var data TokenizedTexts
	if err := c.Bind(&data); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	mu.Lock()
	defer mu.Unlock()

	requestCount++
	ProcessedTokens += uint64(len(data.TokenizedText))

	elapsed := time.Since(startTime).Seconds()
	tps := float64(requestCount) / elapsed

	sample := SaveDataSample{
		Category:       uint8(data.Category),
		NumberOfTokens: uint64(len(data.TokenizedText)),
		TokenizedText:  make([]uint32, len(data.TokenizedText)),
	}

	for i, token := range data.TokenizedText {
		sample.TokenizedText[i] = uint32(token)
	}

	currentFile.Samples[currentIndex] = sample
	currentIndex++

	if currentIndex >= SamplesPerFile {
		if err := os.MkdirAll(PATH_TO_SAVE, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create directory"})
		}

		filePath := filepath.Join(PATH_TO_SAVE, fmt.Sprintf("data_%d.bin", fileCounter))
		if err := writeSaveFileToDisc(&currentFile, filePath); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save file"})
		}

		fileCounter++
		currentIndex = 0
		currentFile = SaveFile{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"current_index":    currentIndex,
		"file_counter":     fileCounter,
		"processed_tokens": ProcessedTokens,
		"requests_per_sec": tps,
		"total_requests":   requestCount,
	})
}

func health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	e.GET("/health", health)
	e.POST("/save-data", saveData)

	e.Logger.Fatal(e.Start(":4567"))
}

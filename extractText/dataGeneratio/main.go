package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

type BatchRequest struct {
	BatchSize       int `json:"batch_size"`
	MaxSampleLength int `json:"max_sample_length"`
}

type BatchSample struct {
	InputSeq  []int32   `json:"input_seq"`
	TargetSeq []int32   `json:"target_seq"`
	Mask      []float32 `json:"mask"`
}

type BatchResponse struct {
	Samples     []BatchSample `json:"samples"`
	TotalTokens uint64        `json:"total_tokens"`
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

	mathrand.Seed(time.Now().UnixNano())
	randomFile := files[mathrand.Intn(len(files))]

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

	sampleCache     []CachedSample
	cacheMu         sync.RWMutex
	cacheSize       = 256 * 512
	refillThreshold = 256 * 16
	isRefilling     int32

	fileListCache  []string
	fileListMu     sync.RWMutex
	lastFileUpdate time.Time

	refillChan = make(chan int, 10)
)

type CachedSample struct {
	TokenizedText []uint32
	Category      uint8
}

func init() {
	startTime = time.Now()
	mathrand.Seed(time.Now().UnixNano())
	initializeCounters()
	updateFileList()

	fmt.Println("Initializing sample cache...")
	fillCache(cacheSize)
	fmt.Printf("Cache initialized with %d samples\n", len(sampleCache))

	go cacheRefillWorker()
}

func initializeCounters() {
	files, err := filepath.Glob(filepath.Join(PATH_TO_SAVE, "*.bin"))
	if err != nil {
		return
	}

	fileCounter = len(files)

	var totalSize int64
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		totalSize += info.Size()
	}

	const tokenSize = 4
	ProcessedTokens = uint64(totalSize / tokenSize)
}

func generateRandomID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 16)
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)

	for i := range result {
		result[i] = charset[randomBytes[i]%byte(len(charset))]
	}

	return string(result)
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

		randomID := generateRandomID()
		filePath := filepath.Join(PATH_TO_SAVE, fmt.Sprintf("data_%s.bin", randomID))
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

func updateFileList() {
	files, err := filepath.Glob(filepath.Join(PATH_TO_SAVE, "*.bin"))
	if err != nil {
		return
	}

	fileListMu.Lock()
	fileListCache = files
	lastFileUpdate = time.Now()
	fileListMu.Unlock()
}

func getFileList() []string {
	fileListMu.RLock()
	defer fileListMu.RUnlock()

	if time.Since(lastFileUpdate) > 30*time.Second {
		go updateFileList()
	}

	return fileListCache
}

func loadRandomSample() (*CachedSample, error) {
	files := getFileList()
	if len(files) == 0 {
		return nil, fmt.Errorf("no save files found")
	}

	randomFile := files[mathrand.Intn(len(files))]
	saveFile, err := readSaveFileFromDisc(randomFile)
	if err != nil {
		return nil, err
	}

	randomSampleIdx := mathrand.Intn(SamplesPerFile)
	sample := &saveFile.Samples[randomSampleIdx]

	if sample.NumberOfTokens < 2 {
		return nil, fmt.Errorf("sample too short")
	}

	cachedSample := &CachedSample{
		TokenizedText: make([]uint32, len(sample.TokenizedText)),
		Category:      sample.Category,
	}
	copy(cachedSample.TokenizedText, sample.TokenizedText)

	return cachedSample, nil
}

func fillCache(count int) {
	files := getFileList()
	if len(files) == 0 {
		fmt.Println("No data files found")
		return
	}

	filesNeeded := (count + SamplesPerFile - 1) / SamplesPerFile
	if filesNeeded > len(files) {
		filesNeeded = len(files)
	}

	type loadResult struct {
		samples []CachedSample
		fileIdx int
	}

	resultChan := make(chan loadResult, filesNeeded)
	var wg sync.WaitGroup

	workers := 8
	if filesNeeded < workers {
		workers = filesNeeded
	}

	filesToLoad := make([]string, 0, filesNeeded)
	for i := 0; i < filesNeeded; i++ {
		filesToLoad = append(filesToLoad, files[mathrand.Intn(len(files))])
	}

	fileIndex := 0
	var indexMu sync.Mutex

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				indexMu.Lock()
				if fileIndex >= len(filesToLoad) {
					indexMu.Unlock()
					return
				}
				idx := fileIndex
				fileIndex++
				indexMu.Unlock()

				saveFile, err := readSaveFileFromDisc(filesToLoad[idx])
				if err != nil {
					continue
				}

				var fileSamples []CachedSample
				for j := 0; j < SamplesPerFile; j++ {
					sample := &saveFile.Samples[j]
					if sample.NumberOfTokens < 2 {
						continue
					}

					cachedSample := CachedSample{
						TokenizedText: sample.TokenizedText,
						Category:      sample.Category,
					}
					fileSamples = append(fileSamples, cachedSample)
				}

				resultChan <- loadResult{samples: fileSamples, fileIdx: idx}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var newSamples []CachedSample
	filesLoaded := 0
	for result := range resultChan {
		newSamples = append(newSamples, result.samples...)
		filesLoaded++
	}

	cacheMu.Lock()
	sampleCache = append(sampleCache, newSamples...)
	finalSize := len(sampleCache)
	cacheMu.Unlock()

	avgPerFile := 0
	if filesLoaded > 0 {
		avgPerFile = len(newSamples) / filesLoaded
	}
	fmt.Printf("Cache refill: %d files -> %d samples (%d avg/file, total: %d)\n", 
		filesLoaded, len(newSamples), avgPerFile, finalSize)
}

func cacheRefillWorker() {
	for needed := range refillChan {
		if atomic.CompareAndSwapInt32(&isRefilling, 0, 1) {
			cacheMu.RLock()
			currentSize := len(sampleCache)
			cacheMu.RUnlock()

			if currentSize < refillThreshold {
				toAdd := cacheSize - currentSize
				if toAdd > needed {
					toAdd = needed
				}
				if toAdd > 0 {
					fillCache(toAdd)
				}
			}
			atomic.StoreInt32(&isRefilling, 0)
		}
	}
}

func getSampleFromCache() (*CachedSample, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if len(sampleCache) == 0 {
		return nil, fmt.Errorf("cache is empty")
	}

	idx := mathrand.Intn(len(sampleCache))
	sample := sampleCache[idx]

	sampleCache[idx] = sampleCache[len(sampleCache)-1]
	sampleCache = sampleCache[:len(sampleCache)-1]

	currentSize := len(sampleCache)
	if currentSize < refillThreshold && currentSize > 0 {
		select {
		case refillChan <- cacheSize - currentSize:
		default:
		}
	}

	return &sample, nil
}

func prepareSequence(tokenized []uint32, maxSampleLength int) ([]int32, []int32, []float32, bool) {
	if len(tokenized) < 2 {
		return nil, nil, nil, false
	}

	var chunk []uint32
	if len(tokenized) <= maxSampleLength+1 {
		chunk = tokenized
	} else {
		startIdx := mathrand.Intn(len(tokenized) - maxSampleLength - 1)
		chunk = tokenized[startIdx : startIdx+maxSampleLength+1]
	}

	inputSeq := make([]int32, maxSampleLength)
	targetSeq := make([]int32, maxSampleLength)
	mask := make([]float32, maxSampleLength)

	chunkLen := len(chunk) - 1
	padLen := maxSampleLength - chunkLen

	for i := 0; i < padLen; i++ {
		inputSeq[i] = 0
		targetSeq[i] = 0
		mask[i] = 0.0
	}

	for i := 0; i < chunkLen; i++ {
		inputSeq[padLen+i] = int32(chunk[i])
		targetSeq[padLen+i] = int32(chunk[i+1])
		mask[padLen+i] = 1.0
	}

	return inputSeq, targetSeq, mask, true
}

func getBatch(c echo.Context) error {
	var req BatchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.BatchSize <= 0 || req.MaxSampleLength <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "batch_size and max_sample_length must be positive"})
	}

	samples := make([]BatchSample, 0, req.BatchSize)
	var totalTokens uint64

	for len(samples) < req.BatchSize {
		cachedSample, err := getSampleFromCache()
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		inputSeq, targetSeq, mask, ok := prepareSequence(cachedSample.TokenizedText, req.MaxSampleLength)
		if !ok {
			continue
		}

		var nonPaddingTokens uint64
		for _, m := range mask {
			if m > 0.0 {
				nonPaddingTokens++
			}
		}

		samples = append(samples, BatchSample{
			InputSeq:  inputSeq,
			TargetSeq: targetSeq,
			Mask:      mask,
		})

		totalTokens += nonPaddingTokens
	}

	response := BatchResponse{
		Samples:     samples,
		TotalTokens: totalTokens,
	}

	return c.JSON(http.StatusOK, response)
}

func cacheStats(c echo.Context) error {
	cacheMu.RLock()
	currentSize := len(sampleCache)
	cacheMu.RUnlock()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"cache_size":        currentSize,
		"cache_capacity":    cacheSize,
		"refill_threshold":  refillThreshold,
		"is_refilling":      atomic.LoadInt32(&isRefilling) == 1,
		"cache_utilization": float64(currentSize) / float64(cacheSize) * 100,
	})
}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	e.GET("/health", health)
	e.POST("/save-data", saveData)
	e.POST("/get-batch", getBatch)
	e.GET("/cache-stats", cacheStats)

	e.Logger.Fatal(e.Start(":4567"))
}

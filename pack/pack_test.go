package pack

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/zstd"
)

const (
	test_max_input_size_bytes = 10 * 1024 * 1024
	// size guaranteed to fit worstcase compression in all tests
	test_compression_bound_bytes = 2*test_max_input_size_bytes + 1000
	path_loghubCorpus            = "./../testData/loghubCorpus/"
	path_corruptedCorpus         = "./../testData/unpackCorruptedCorpus/"
)

var benchmarked_compression_levels = [...]int{4, 9}

func TestPackAndUnpackOnCorpus(t *testing.T) {
	testPackAndUnpackFromDir(t, path_loghubCorpus)
}

func testPackAndUnpackFromDir(t *testing.T, directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		log.Fatal(err)
	}

	inputBuff := make([]byte, test_max_input_size_bytes)
	packedBuff := make([]byte, test_compression_bound_bytes)
	unpackedBuff := make([]byte, test_max_input_size_bytes)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := directory + e.Name() + "/"

		packInputSize := readFileToBuffer(inputBuff, dir+findFirstLogFile(dir))
		t.Run(e.Name(), func(t *testing.T) {
			// --------- packing
			packOutputSize := PackBuffer(inputBuff[:packInputSize], packedBuff, COMPRESSION_LEVEL_DEFAULT)

			// --------- unpacking
			_, unpackOutputSize := Decompress(unpackedBuff, packedBuff[:packOutputSize])

			// --------- test assertions
			assertInversibility(t, e.Name(), inputBuff, unpackedBuff, packInputSize, unpackOutputSize)
		})
	}
}

func findFirstLogFile(path string) string {
	entries, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			return entry.Name()
		}
	}
	panic("Dir with benchmark data is corrupted")
}

func readFileToBuffer(inBuff []byte, path string) (fileSize int) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	for {
		n, err := file.ReadAt(inBuff, int64(fileSize))

		if err != nil && err != io.EOF {
			log.Fatal(err)
		}

		fileSize += n

		if err == io.EOF {
			break
		}
	}
	return
}

func PackBuffer(fileContent, outBuff []byte, compressionLevel int) (totalBytesWritten int) {
	for read, written := 0, 0; len(fileContent) > 0; {
		read, written = Compress(outBuff, fileContent, compressionLevel)

		fileContent = fileContent[read:]
		outBuff = outBuff[written:]
		totalBytesWritten += written
	}
	return
}

func UnpackBuffer(packedBuffer, outBuff []byte, t *testing.T) int {
	read, written := Decompress(outBuff, packedBuffer)

	if read != len(packedBuffer) {
		t.Fatalf("Unpacked only %d bytes of %d buffer!", read, len(packedBuffer))
	}
	return written
}

func assertInversibility(t *testing.T, name string, inputBuff, unpackedBuff []byte, inputSize, unpackedSize int) {
	if unpackedSize != inputSize {
		t.Errorf("Size of pack-unpack output is different than size of input! in: %d; out: %d, file: %s",
			inputSize, unpackedSize, name)
		return
	}
	inputBuff = inputBuff[:inputSize]
	unpackedBuff = unpackedBuff[:unpackedSize]

	for i, inByte := range inputBuff {
		if inByte != unpackedBuff[i] {

			relevantInput := inputBuff[lowerMargin(i) : i+1]
			relevantUnpacked := unpackedBuff[lowerMargin(i) : i+1]

			t.Errorf(`Decompressed data does not match the original at idx: %d!

in raw : %v
out raw: %v

in as string : "%s"
out as string: "%s"
`, i, relevantInput, relevantUnpacked,
				string(relevantInput), string(relevantUnpacked))
			return
		}
	}
}

func lowerMargin(a int) int {
	printMargin := 100
	a -= printMargin
	if a >= 0 {
		return a
	}
	return 0
}


func TestGracefullyFailUnpackingCorruptedArchives(t *testing.T) {
	entries, err := os.ReadDir(path_corruptedCorpus)
	if err != nil {
		log.Fatal(err)
	}

	packedBuff := make([]byte, test_compression_bound_bytes)
	unpackedBuff := make([]byte, test_max_input_size_bytes)

	for _, e := range entries {
		path := path_corruptedCorpus + e.Name()

		unpackInputSize := readFileToBuffer(packedBuff, path)
		t.Run(e.Name(), func(t *testing.T) {
			// ---------try to unpack
			bytesRead, _ := Decompress(unpackedBuff, packedBuff[:unpackInputSize])

			if bytesRead != CORRUPT_INPUT {
				t.Errorf("Failed to detect corrupted *.lp archive! file: %s", e.Name())
				return
			}
		})
	}
}

////////////////////

func BenchmarkPacking(b *testing.B) {

	entries, err := os.ReadDir(path_loghubCorpus)
	if err != nil {
		log.Fatal(err)
	}

	inputBuff := make([]byte, test_max_input_size_bytes)
	packedBuff := make([]byte, test_max_input_size_bytes)
	unpackedBuff := make([]byte, test_max_input_size_bytes)

	for _, compressionLevel := range benchmarked_compression_levels {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := path_loghubCorpus + e.Name() + "/"
			packInputSize := readFileToBuffer(inputBuff, dir+findFirstLogFile(dir))
			var packOutputSize int

			// --------- benchmark packing
			level_str := "_level_" + strconv.Itoa(compressionLevel) + "_"
			b.Run("pack" + level_str+e.Name(), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					// report MB/s
					b.SetBytes(int64(packInputSize))
					packOutputSize = PackBuffer(inputBuff[:packInputSize], packedBuff, compressionLevel)

				}
				b.ReportMetric(float64(packInputSize)/float64(packOutputSize), "compRatio")
			})
			b.Run("unpack" + level_str + e.Name(), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					b.SetBytes(int64(packOutputSize))
					Decompress(unpackedBuff, packedBuff[:packOutputSize])
				}
			})
		}
	}
}

func BenchmarkVsZstd(b *testing.B) {
	entries, err := os.ReadDir(path_loghubCorpus)
	if err != nil {
		log.Fatal(err)
	}

	inputBuff        := make([]byte, test_max_input_size_bytes)
	packedStage1Buff := make([]byte, test_max_input_size_bytes)
	packedStage2Buff := make([]byte, test_max_input_size_bytes)

	var totalInputSize           int64
	var totalZstdCompressedSize  int64
	var totalLp4ZstdompressedSize int64
	var totalLp9ZstdompressedSize int64

	for _,e := range entries {
		if !e.IsDir() {
			continue
		}

		
		var packStage1OutputSize int
		var ratio_zstd float64

		dir := path_loghubCorpus + e.Name() + "/"
		packInputSize := readFileToBuffer(inputBuff, dir+findFirstLogFile(dir))
		totalInputSize += int64(packInputSize)

		b.Run("zstd_" + e.Name(), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// report MB/s
				b.SetBytes(int64(packInputSize))

				zstdResultBuff, err := zstd.Compress(packedStage1Buff, inputBuff[:packInputSize])
				if err != nil {
					b.Fatalf("Zstd compression failed! %v", err)
				}
				packStage1OutputSize = len(zstdResultBuff)
			}

			ratio_zstd = float64(packInputSize)/float64(packStage1OutputSize)
			b.ReportMetric(ratio_zstd, "compRatio")
		})
		totalZstdCompressedSize += int64(packStage1OutputSize)

		for _, compressionLevel := range benchmarked_compression_levels {

			levelStr := "_level_" + strconv.Itoa(compressionLevel) + "_"

			var packStage2OutputSize int
			
			b.Run("lp+zstd" + levelStr+e.Name(), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					// report MB/s
					b.SetBytes(int64(packInputSize))

					packStage1OutputSize         = PackBuffer(inputBuff[:packInputSize], packedStage1Buff, compressionLevel)
					packedStage2BuffResult, err := zstd.Compress(packedStage2Buff, packedStage1Buff[:packStage1OutputSize])
					if err != nil {
						b.Fatalf("Zstd compression failed! %v", err)
					}
					packStage2OutputSize = len(packedStage2BuffResult)
				}
				ratio_lp_zstd := float64(packInputSize)/float64(packStage2OutputSize) 
				b.ReportMetric(ratio_lp_zstd, "compRatio")
				b.ReportMetric(ratio_lp_zstd/ratio_zstd, "RatioImprovement")
			})
			if compressionLevel == 4 {
				totalLp4ZstdompressedSize += int64(packStage2OutputSize)
			} else if compressionLevel == 9 {
				totalLp9ZstdompressedSize += int64(packStage2OutputSize)
			} else {
				b.Fatalf("Please update test to handle other compression ratios")
			}
		}
	}
	fmt.Println("-------------------------------")
	b.Run("avg_total", func(b *testing.B) {
		totalInputF    := float64(totalInputSize)
		avgZstdRatio   := totalInputF/float64(totalZstdCompressedSize) 
		avgLp4ZstdRatio := totalInputF/float64(totalLp4ZstdompressedSize)
		avgLp9ZstdRatio := totalInputF/float64(totalLp9ZstdompressedSize)

		b.ReportMetric(avgZstdRatio,                 "avgZstdRatio")
		
		b.ReportMetric(avgLp4ZstdRatio,              "avgLp4+ZstdRatio")
		b.ReportMetric(avgLp4ZstdRatio/avgZstdRatio, "avgLp4RatioImprovement")
		b.ReportMetric(avgLp9ZstdRatio,              "avgLp9+ZstdRatio")
		b.ReportMetric(avgLp9ZstdRatio/avgZstdRatio, "avgLp9RatioImprovement")
	})
	
}

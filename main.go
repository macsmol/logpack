package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"macsmol.pl/logpack/pack"
)

const (
	MAX_DISK_READ_BYTES = 5 * 1000 * 1000
)

func main() {
	if len(os.Args) == 2 {
		tryDoPack(os.Args[1], pack.COMPRESSION_LEVEL_DEFAULT)
	} else if len(os.Args) == 3 {
		if os.Args[1] == "-d" {
			flp := openFileForReadingOrDie(os.Args[2])
			defer flp.Close()

			outputFileName := deriveOutputFileNameOrDie(os.Args[2])
			
			unpackedFile := createFileForWritingOrDie(outputFileName, "Cannot unpack %v")
			defer unpackedFile.Close()

			start := time.Now()
			totalBytesRead, totalBytesWritten := unpackFile(flp, unpackedFile)

			{
				elapsed := time.Since(start)

				var megabytesRead  float32   = float32(totalBytesRead)    / 1000_000.0
				var megabytesWritten float32 = float32(totalBytesWritten) / 1000_000.0
				var speed_MBps float32 = float32(totalBytesRead) / float32(elapsed.Microseconds())

				fmt.Printf("%.2f MB unpacked to %.2f MB in %.2fs (%5.2f MB/s)\n", 
				           megabytesRead, megabytesWritten, elapsed.Seconds(), speed_MBps)
			}

		} else if compressionLevel, err := tryToParseCompressionLevel(os.Args[1]); err == nil {
			tryDoPack(os.Args[2], compressionLevel)
		} else {
			printUsageAndExit()
		}
	} else {
		printUsageAndExit()
	}
}

func deriveOutputFileNameOrDie(inputFilename string) string {
	outputFileName, suffixFound := strings.CutSuffix(inputFilename, ".lp")
	if !suffixFound {
		fmt.Printf("Unknown file extension (.lp expected). Ignoring.\n")
		os.Exit(0)
	} 
	return outputFileName
}

func openFileForReadingOrDie(filePath string) *os.File {
	flp, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Default().Fatalf("Cannot open %s. File does not exist\n", filePath)
		}
		log.Default().Fatalf("Cannot open: %v", err)
	}
	return flp
}

func createFileForWritingOrDie(outputFileName, fmtString string) *os.File {
	var file *os.File
	var err error
	file, err = os.OpenFile(outputFileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			fmt.Printf("File %s already exists. Overwrite (y/n) ? ", outputFileName)

			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			text := scanner.Text()

			if text == "y" {
				file, err = os.Create(outputFileName)
				if err != nil {
					log.Default().Fatalf(fmtString, err)
				}
			} else {
				fmt.Printf("Not overwritten\n")
				os.Exit(0)
			}
		} else {
			log.Default().Fatalf(fmtString, err)
		}
	}
	return file
}

func tryDoPack(inputFilePath string, compressionLevel int) {
	//------------------ OPEN raw log file
	f := openFileForReadingOrDie(inputFilePath)
	defer f.Close()

	//------------------  CREATE packed log file
	outputFileName := inputFilePath + ".lp"
	flp := createFileForWritingOrDie(outputFileName, "Cannot unpack %v")
	defer flp.Close()

	start := time.Now()
	totalBytesRead, totalBytesWritten := packFile(f, flp, compressionLevel)

	{
		elapsed := time.Since(start)
		var megabytesRead float32 = float32(totalBytesRead) / 1000_000.0
		var megabytesWritten float32 = float32(totalBytesWritten) / 1000_000.0
		var compRatioPercent float32 = float32(100*totalBytesWritten) / float32(totalBytesRead)
		
		var speed_MBps float32 = float32(totalBytesRead) / float32(elapsed.Microseconds())
		fmt.Printf("(%s => %s) %.2f MB packed to %.2f MB (%.1f%%) in %.2fs; average speed: %.1f MB/s\n",
		           inputFilePath, outputFileName, 
				   megabytesRead, megabytesWritten, compRatioPercent, 
				   elapsed.Seconds(), speed_MBps)
	}
}

func tryToParseCompressionLevel(arg string) (int, error) {

	if len(arg) != 2 || arg[0] != '-' {
		return -1, errors.New("cannot parse compression level")
	}
	return strconv.Atoi(arg[1:])
}

func printUsageAndExit() {
	fmt.Printf(`Usage is:

	Packing:
logpack [Options.. ] file.log

	Unpacking:
logpack -d file.lp

Options:
   -#       Desired compression level, where '#' is a number between 1 and 9;
            lower numbers provide faster compression, higher numbers yield
            better compression ratios. [Default: 4]
`)
	os.Exit(0)
}

func packFile(inFile, outFile *os.File, compressionLevel int) (totalBytesRead, totalBytesWritten int64) {
	fi, err := inFile.Stat()
	if err != nil {
		log.Fatal(err)
	}
	inputFileSizeBytes := fi.Size()

	chunkSize := pack.DecompressBound()
	inBuff := make([]byte, MAX_DISK_READ_BYTES)
	outBuff := make([]byte, chunkSize)

	for {
		n, err := inFile.ReadAt(inBuff, totalBytesRead)

		if err != nil && err != io.EOF {
			log.Fatal(err)
		}

		inRemainder := inBuff[:n]
		// write compressed until input buffer is read completely.
		for len(inRemainder) > 0 {
			read, written := pack.Compress(outBuff, inRemainder, compressionLevel)

			_, err2 := outFile.Write(outBuff[:written])
			if err2 != nil {
				log.Fatal(err2)
			}

			inRemainder = inRemainder[read:]

			totalBytesWritten += int64(written)
		}
		totalBytesRead += int64(n)

		{
			var megabytesRead float32 = float32(totalBytesRead) / 1000_000.0
			var inputMegabytes float32 = float32(inputFileSizeBytes) / 1000_000.0
			var compRatioPercent float32 = float32(100*totalBytesWritten) / float32(totalBytesRead)

			fmt.Printf("%7.2f MB / %.2f MB packed (%.1f%%)\r", 
			           megabytesRead, inputMegabytes, compRatioPercent)
		}

		if err == io.EOF {
			break
		}
	}
	return
}

func unpackFile(packed, dstFile *os.File) (totalBytesRead, totalBytesWritten int64) {
	fi, err := packed.Stat()
	if err != nil {
		log.Fatal(err)
	}
	inputFileSizeBytes := fi.Size()

	inBuff := make([]byte, MAX_DISK_READ_BYTES)
	unpackedBuff := make([]byte, pack.DecompressBound())

	for {
		n, err := packed.ReadAt(inBuff, totalBytesRead)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}

		inRemainder := inBuff[:n]
		// write decompressed until input buffer is read completely
		for len(inRemainder) > 0 {
			compressedBytesRead, uncompressedBytesWritten := pack.Decompress(unpackedBuff, inRemainder)

			if compressedBytesRead == pack.CORRUPT_INPUT {
				log.Fatalf("Error: Cannot unpack \"%s\". Input file is corrupted or is not a Logpack archive\n", packed.Name())
			}

			// inRemainder did not contain full chunk; break to read more from disk on fresh buffer
			if compressedBytesRead == pack.NOT_ENOUGH_INPUT {
				// header declares that there is more input but we're at the end
				if err == io.EOF {
					log.Fatalf("Error: Cannot unpack \"%s\". Input file is corrupted or is not a Logpack archive\n", packed.Name())
				}
				break
			}
			inRemainder = inRemainder[compressedBytesRead:]

			totalBytesRead    += int64(compressedBytesRead)
			totalBytesWritten += int64(uncompressedBytesWritten)

			_, err2 := dstFile.Write(unpackedBuff[:uncompressedBytesWritten])
			if err2 != nil {
				log.Fatal(err2)
			}
		}

		{
			var megabytesRead  float32 = float32(totalBytesRead)     / 1000_000.0
			var inputMegabytes float32 = float32(inputFileSizeBytes) / 1000_000.0
			fmt.Printf("%.2f MB / %.2f MB unpacked\r", megabytesRead, inputMegabytes)
		}

		if err == io.EOF {
			break
		}
	}
	return 
}

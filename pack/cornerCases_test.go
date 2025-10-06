package pack

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

const (
	number_of_random_cases = 50
	dict_size              = 80

	abnormal_inputs_dir                = "./../testData/abnormalCases/"
)

func TestPackAndUnpackLongLines(t *testing.T) {

	packedBuff := make([]byte, test_compression_bound_bytes)
	unpackedBuff := make([]byte, test_max_input_size_bytes)

	testSeed := time.Now().UnixMicro()
	for i := 0; i < number_of_random_cases; i++ {
		inputBuff := randomTextWithLongLines(testSeed)

		t.Run(fmt.Sprintf("seed %d", testSeed), func(t *testing.T) {
			// --------- packing
			packOutputSize := PackBuffer(inputBuff, packedBuff, COMPRESSION_LEVEL_DEFAULT)

			// --------- unpacking
			unpackOutputSize := UnpackBuffer(packedBuff[:packOutputSize], unpackedBuff, t)

			// --------- test assertions
			assertInversibility(t, fmt.Sprintf("seed %d", testSeed), inputBuff, unpackedBuff, len(inputBuff), unpackOutputSize)
		})
		testSeed++
	}
}

func TestPackAndUnpackNonAsciiLines(t *testing.T) {

	packedBuff := make([]byte, test_compression_bound_bytes)
	unpackedBuff := make([]byte, test_max_input_size_bytes)

	testSeed := time.Now().UnixMicro()
	for i := 0; i < number_of_random_cases; i++ {
		inputBuff := randomNonAsciiLines(testSeed)
		// storeToDebugFile("nonAscii", inputBuff)

		t.Run(fmt.Sprintf("seed %d", testSeed), func(t *testing.T) {
			// --------- packing
			packOutputSize := PackBuffer(inputBuff, packedBuff, COMPRESSION_LEVEL_DEFAULT)

			// --------- unpacking
			unpackOutputSize := UnpackBuffer(packedBuff[:packOutputSize], unpackedBuff, t)

			// --------- test assertions
			assertInversibility(t, fmt.Sprintf("seed %d", testSeed), inputBuff, unpackedBuff, len(inputBuff), unpackOutputSize)
		})
		testSeed++
	}
}

func TestCompressEmpty(t * testing.T) {
	packedBuff := make([]byte, test_compression_bound_bytes)
	unpackedBuff := make([]byte, test_max_input_size_bytes)

	inputBuff := make([]byte, 0)

	t.Run("Compressing empty input", func(t *testing.T) {
		// --------- packing
		read, written := Compress(packedBuff, inputBuff, COMPRESSION_LEVEL_DEFAULT)
		if read != 0 {
			t.Errorf("Compression of empty buffer read non-zero bytes: %d", read)
		}
		_, writtenDec := Decompress(unpackedBuff,packedBuff[:written])
		if writtenDec != 0 {
			t.Errorf("Empty buffer after pack-unpack is no longer empty! and has %d bytes", writtenDec)
		}
	})
}

func randomTextWithLongLines(seed int64) []byte {
	r := rand.New(rand.NewSource(seed))

	var randomWords [dict_size][]byte = initRandomDict(r)

	rawBuff := make([]byte, 0, test_max_input_size_bytes)
	sizeLimit := cap(rawBuff)

	wordsSinceNewLine := 0
	for {
		randomWord := randomWords[r.Int31n(dict_size)]
		if len(rawBuff)+len(randomWord)+2 > sizeLimit {
			break
		}
		rawBuff = append(rawBuff, randomWord...)

		//separate words by spaces
		rawBuff = append(rawBuff, []byte(" ")...)

		// very long lines (often longer than MAX_CHUNK_SIZE)
		wordsSinceNewLine++
		if wordsSinceNewLine > 5000 && r.Int()%1000 == 0 {
			rawBuff = append(rawBuff, []byte("\n")...)
			wordsSinceNewLine = 0
		}
	}
	rawBuff = append(rawBuff, []byte("\n")...)

	return rawBuff
}

func randomNonAsciiLines(seed int64) []byte {
	r := rand.New(rand.NewSource(seed))

	rawBuff := make([]byte, test_max_input_size_bytes)

	const SIZEOF_INT_64 = 8
	for i := 0; i < len(rawBuff); i += SIZEOF_INT_64 {
		binary.LittleEndian.PutUint64(rawBuff[i:], r.Uint64())
		rawBuff[i  ] |= 0x80
		rawBuff[i+1] |= 0x80
		rawBuff[i+2] |= 0x80
		rawBuff[i+3] |= 0x80
		rawBuff[i+4] |= 0x80
		rawBuff[i+5] |= 0x80
		rawBuff[i+6] |= 0x80
		rawBuff[i+7] |= 0x80
	}
	for i := 80; i < len(rawBuff); i += r.Intn(80) + 80 {
		rawBuff[i] = '\n'
	}
	return rawBuff
}

func initRandomDict(r *rand.Rand) [dict_size][]byte {
	myEncoding := base64.StdEncoding.WithPadding(base64.NoPadding)
	var words [dict_size][]byte
	for i, word := range words {
		someBytes := make([]byte, 0, 2+r.Int31n(8))

		length := cap(someBytes)
		for j := 0; j < length; j++ {
			someBytes = append(someBytes, byte(r.Int()))
		}
		word = make([]byte, 30)
		myEncoding.Encode(word, someBytes)

		outputLen := myEncoding.EncodedLen(len(someBytes))
		words[i] = word[0:outputLen]

	}

	return words
}

func TestPackAndUnpackAbnormalInputs(t *testing.T) {
	testPackAndUnpackFromDir(t, abnormal_inputs_dir)
}

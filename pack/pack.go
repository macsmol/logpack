package pack

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// error that may be happen during by Decompress()
const (
	NOT_ENOUGH_INPUT        int = -1
	NOT_ENOUGH_OUTPUT_SPACE int = -2
	CORRUPT_INPUT           int = -3
)

const (
	// Used to denote something else than a literal charatcer in the compressed buffer.
	// Exact meaning varies depending on context (beginning of line, middle of line, after another escape byte)
	ESCAPE_BYTE byte = 128 // 0x80
	// In compressed buffer this flag be set in the byte that encodes linesBefore.
	// If set it means that encoded number that follows immediately encodes initial offset in keyLine rather than prefix length.
	NO_SHARED_PREFIX_FLAG byte = 0x40
	// LENGTH_BASE - 1 is maximum length that can be encoded in one byte
	LENGTH_BASE byte = 127
	// how many previous lines can be used for comparing current line; higher number means higher compression ratio;
	MAX_BACKREFERENCE_CAPACITY = 64

	SIZEOF_INT16 = 2
	HEADER_SIZE  = 2 * SIZEOF_INT16
	// Max buffer size that can be compressed in one Compress() call. Also max size of x
	// that can be stored in 2-byte var. No need to stored empty buffers so 0 means 1
	MAX_CHUNK_SIZE = math.MaxUint16 + 1

	// limit to how many chars of line are considered in similarity score
	MAX_SIMILARITY = 140
)

const (
	COMPRESSION_LEVEL_WORST   int = 1
	COMPRESSION_LEVEL_BEST    int = 9
	COMPRESSION_LEVEL_DEFAULT int = 4
)

type compressionParameters struct {
	backreferenceCapacity byte
	goodEnoughFactor      float32
}

var compressionLevelPresets = [...]compressionParameters{
	{2, 0.80},  // pad to align levels to 1-9 range;
	{2, 0.80},  // CompressionLevel 1
	{4, 0.80},  // CompressionLevel 2
	{8, 0.80},  // CompressionLevel 3
	{16, 0.80}, // CompressionLevel 4 <-The Default
	{32, 0.80}, // CompressionLevel 5
	{64, 0.80}, // CompressionLevel 6
	{64, 0.90}, // CompressionLevel 7
	{64, 0.95}, // CompressionLevel 8
	{64, 1.00}, // CompressionLevel 9
}

// var debug_LinePacked = 1

type lineReference struct {
	line            []byte
	linesBefore     byte
	prefixLength    int
	similarityScore int
}

// Cyclic buffer of previously read lines. Next line will be stored at writeIdx index.
type backrefBuffer struct {
	writeIdx      int
	oldestLineIdx int
	capacity      int
	lines         [MAX_BACKREFERENCE_CAPACITY][]byte
}

func (backref *backrefBuffer) add(line []byte) {
	backref.lines[backref.writeIdx] = line
	backref.writeIdx++
	backref.writeIdx %= backref.capacity
	// max capacity reached - remove oldest line
	if backref.writeIdx == backref.oldestLineIdx {
		backref.oldestLineIdx++
		backref.oldestLineIdx %= backref.capacity
	}
}

// finds a line with longest prefix shared with compressedLine. Returns it along with info lines before it was encountered (eg. 1 for previous line)
func (backref *backrefBuffer) chooseReferenceLine(compressedLine []byte, goodEnoughFactor float32) (lineRef lineReference) {
	// don't refer current line (0). refer at least previous line
	lineRef.linesBefore = 1

	goodEnoughSimilarityScore := goodEnoughFactor * float32(min2(len(compressedLine),
		MAX_SIMILARITY))

	for linesBefore := 1; ; linesBefore++ {
		i := backref.writeIdx - linesBefore
		// wrap around
		if i < 0 {
			i = backref.capacity + i
		}

		prefixLength, similarity := estimateSimilarity(backref.lines[i], compressedLine)
		if similarity > lineRef.similarityScore {
			lineRef.linesBefore = byte(linesBefore)
			lineRef.line = backref.lines[i]
			lineRef.prefixLength = prefixLength
			lineRef.similarityScore = similarity
			if float32(similarity) >= goodEnoughSimilarityScore {
				break
			}
		}

		// reached the end of buffer
		// watch out! - will see empty buff as full. Not important if we never read empty buff
		if i == backref.oldestLineIdx {
			break
		}
	}
	return
}

func (backref *backrefBuffer) getLineAt(linesBefore int) []byte {
	if linesBefore > backref.capacity {
		panic(fmt.Sprintf("Trying to reference a line outside of BACKREFERENCE_CAPACITY: %d", linesBefore))
	}
	i := backref.writeIdx - linesBefore
	// wrap around
	if i < 0 {
		i = backref.capacity + i
	}
	return backref.lines[i]
}

func limitSlice(slice []byte, lengthLimit int) []byte {
	if len(slice) > lengthLimit {
		slice = slice[:lengthLimit]
	}
	return slice
}

// Returns length of prefix and similarity between refLine and currLine.
// Mind that commonPrefixLength has other meaning if it's negative.
// Negative prefix means there is no common prefix. Instead it denotes a starting offset (its negative) to keyLine
// when later compressing a currLine in func compressLine(). Eg. if commonPrefixLength = -2 then first common sequence
// shared by two lines will start at keyLine[2].
func estimateSimilarity(refLine, currLine []byte) (commonPrefixLength, similarityScore int) {
	lenLimit := min3(len(refLine), len(currLine), MAX_SIMILARITY)

	refLine = limitSlice(refLine, lenLimit)
	currLine = limitSlice(currLine, lenLimit)

	for i, char := range refLine {
		if i >= len(currLine) {
			break
		}
		if char == currLine[i] {
			commonPrefixLength++
		} else {
			break
		}
	}

	// Done with prefix.
	// Now estaimate similarity by comparing respective words in a and b up to a idx limit.
	idxRefLine := indexOfFirstSpace(int(commonPrefixLength), refLine)
	idxCurrLine := indexOfFirstSpace(int(commonPrefixLength), currLine)

	similarityScore = commonPrefixLength
	sameStringLength := 0

	// Which means: similarity > 0 but there is no shared prefix, i.e. there will be some references in currLine
	if commonPrefixLength == 0 && idxRefLine < len(refLine) && idxCurrLine < len(currLine) {
		commonPrefixLength = -idxRefLine
	}
	// loop similar to the one in compressLine()
	for idxRefLine < lenLimit && idxCurrLine < lenLimit {
		if currLine[idxCurrLine] == refLine[idxRefLine] {
			sameStringLength++
			idxCurrLine++
			idxRefLine++
		} else {
			// -- end of common sequence --
			// increase c1. encode common sequence in dst (if there is any)
			similarityScore += sameStringLength
			sameStringLength = 0

			// 2. advance cursors in a and b
			idxRefLine = indexOfFirstSpace(idxRefLine, refLine)
			idxCurrLine = indexOfFirstSpace(idxCurrLine, currLine)
		}
	}
	similarityScore += sameStringLength

	return commonPrefixLength, similarityScore
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func min3(a, b, c int) int {
	var ab int
	if a < b {
		ab = a
	} else {
		ab = b
	}
	if ab < c {
		return ab
	}
	return c
}

func getCompressionParameters(compressionLevel int) compressionParameters {
	var row int
	if compressionLevel < 0 {
		row = 0
	} else if compressionLevel == 0 {
		row = int(COMPRESSION_LEVEL_DEFAULT)
	} else if compressionLevel > COMPRESSION_LEVEL_BEST {
		row = COMPRESSION_LEVEL_BEST
	} else {
		row = compressionLevel
	}
	return compressionLevelPresets[row]
}

func Compress(dst, src []byte, compressionLevel int) (bytesRead, bytesWritten int) {
	// cut header; limit dest size to max storable chunk size
	header, dst := dst[:HEADER_SIZE], dst[HEADER_SIZE:]

	src = limitSlice(src, MAX_CHUNK_SIZE)
	dst = limitSlice(dst, MAX_CHUNK_SIZE)

	// fmt.Printf("Compress(), len(src)=%d\n", len(src))

	// fmt.Printf("l:%d ", debug_LinePacked)
	// debug_LinePacked++
	// if debug_LinePacked%10 == 0 {
	// 	fmt.Println("")
	// }

	compressionParams := getCompressionParameters(compressionLevel)
	backref := backrefBuffer{}
	backref.capacity = int(compressionParams.backreferenceCapacity)

	firstLine, src := nextLine(src)
	backref.add(firstLine)

	bytesRead, bytesWritten = quoteSafely(dst, firstLine)
	dst = dst[bytesWritten:]

	for currLine, src := nextLine(src); len(currLine) > 0; currLine, src = nextLine(src) {
		// stop compression if dst has not enough space for the worst-case compression ratio
		// saving the need to do per-char bounds checking later
		if len(dst) < 2*len(currLine)+2 {
			break
		}
		lineRef := backref.chooseReferenceLine(currLine, compressionParams.goodEnoughFactor)

		compressedLineSize := compressLine(lineRef, currLine, dst)
		dst = dst[compressedLineSize:]

		bytesRead += len(currLine)
		bytesWritten += compressedLineSize

		backref.add(currLine)

		// fmt.Printf("l:%d->%d ", debug_LinePacked, lineRef.linesBefore)
		// debug_LinePacked++
		// if debug_LinePacked%10 == 0 {
		// 	fmt.Println("")
		// }
	}

	storeHeader(header, bytesWritten, bytesRead)
	return bytesRead, bytesWritten + HEADER_SIZE
}

// Compresses currLine and writes it to dst buffer
// lineRef - reference to a key line, to which current line is compared
// currLine - line which will be compressed
// dst - buffer where compressed data is written to
func compressLine(lineRef lineReference, currLine, dst []byte) (bytesWritten int) {
	keyLine := lineRef.line

	// previous line is encoded as ESCAPE_BYTE+1; two lines before ESCAPE_BYTE+2 and so on..
	// ESCAPE_BYTE means 'escape following non-ascii literal' (would be useless to reference curr line)
	dst[0] = lineRef.linesBefore + ESCAPE_BYTE
	bytesWritten++

	// lineRef has info about common prefix so we can use it reuse it here rather than find it again
	var sameStringLength int
	var idxKeyLine, idxCurrLine int

	// there was no common prefix - but there was some similarity - there will be encoded strings
	if lineRef.prefixLength < 0 {
		idxKeyLine = -lineRef.prefixLength
		dst[0] = dst[0] | NO_SHARED_PREFIX_FLAG

		// encode initial offset to keyLine so decompression matches up
		bytesWritten += encodeLength(idxKeyLine, dst, int(bytesWritten))
		sameStringLength = 0
	} else {
		sameStringLength, idxKeyLine, idxCurrLine = lineRef.prefixLength, lineRef.prefixLength, lineRef.prefixLength
	}

	for idxKeyLine < len(keyLine) && idxCurrLine < len(currLine) {
		if currLine[idxCurrLine] == keyLine[idxKeyLine] {
			sameStringLength++
			idxCurrLine++
			idxKeyLine++
		} else {
			// -- end of common sequence --
			// 1. encode common sequence in dst (if there is any)
			bytesWritten += encodeLength(sameStringLength, dst, int(bytesWritten))
			sameStringLength = 0

			// 2. advance cursor in refLine
			idxKeyLine = indexOfFirstSpace(idxKeyLine, keyLine)

			// 3. advance cursor in currLine, copy skipped sequence to dst verbatim.
			idxNextSpaceCurrLine := indexOfFirstSpace(idxCurrLine, currLine)
			bytesWritten += quote(dst[bytesWritten:], currLine[idxCurrLine:idxNextSpaceCurrLine])
			idxCurrLine = idxNextSpaceCurrLine
		}
	}
	// Encode whatever accumulated and copy the remainder of currLine to dst
	bytesWritten += encodeLength(sameStringLength, dst, int(bytesWritten))
	bytesWritten += quote(dst[bytesWritten:], currLine[idxCurrLine:])

	return bytesWritten
}

func quote(dst, src []byte) (bytesWritten int) {
	escapedCharsCount := 0
	for i, char := range src {
		// ASCII char
		if char&ESCAPE_BYTE == 0 {
			dst[i+escapedCharsCount] = char
			// anything else (e.g UTF-8)
		} else {
			dst[i+escapedCharsCount] = ESCAPE_BYTE
			escapedCharsCount++
			dst[i+escapedCharsCount] = char
		}
	}
	return len(src) + escapedCharsCount
}

// Copies src to dst up to len(dst). Every ASCII byte (<128) is copied literally. Other bytes are escaped with ESCAPE_BYTE.
func quoteSafely(dst, src []byte) (bytesRead, bytesWritten int) {
	escapedCharsCount := 0
	for i, char := range src {
		if char&ESCAPE_BYTE == 0 {
			// ASCII char
			dst[i+escapedCharsCount] = char
		} else {
			// anything else (e.g UTF-8)

			// not enough room to fit another escape pair
			if i+escapedCharsCount+2 > len(dst) {
				return i, i + escapedCharsCount
			}
			dst[i+escapedCharsCount] = ESCAPE_BYTE
			escapedCharsCount++
			dst[i+escapedCharsCount] = char
		}
	}
	return len(src), len(src) + escapedCharsCount
}

// Starting from startIdx searches buffer for next space character and returns it's index. Returns len(buffer) if no space was found.
func indexOfFirstSpace(startIdx int, buffer []byte) int {
	i := startIdx
	for ; i < len(buffer); i++ {
		if buffer[i] == ' ' {
			return i
		}
	}
	return i
}

// Encodes some integer number in dst at offset.
//
//	The number l is encoded as a single byte
//	with value 128+l, for every l smaller than 127, or a sequence of m bytes with value
//	255 followed by a single byte with value 128+n, for every l not smaller than 127,
//	where l = 127*m + n.
func encodeLength(sameStringLength int, dst []byte, offset int) (bytesWritten int) {
	if sameStringLength == 0 {
		return 0
	}
	bigLengthBytesCount := sameStringLength / int(LENGTH_BASE)

	if bigLengthBytesCount == 0 {
		dst[offset] = ESCAPE_BYTE | byte(sameStringLength)
		return 1
	}
	for i := 0; i < bigLengthBytesCount; i++ {
		dst[offset+i] = ESCAPE_BYTE | LENGTH_BASE
	}
	dst[offset+bigLengthBytesCount] = ESCAPE_BYTE | byte(sameStringLength%int(LENGTH_BASE))
	return bigLengthBytesCount + 1
}

// Reads length encoded at the beginning of compressed slice.
// Function complementary to encodeLength(). See the doc of that function
func decodeLength(compressed []byte) (sameStringLength int, bytesRead int) {
	for _, bite := range compressed {
		sameStringLength += int(bite - ESCAPE_BYTE)
		bytesRead++
		if bite < ESCAPE_BYTE|LENGTH_BASE {
			break
		}
	}
	return sameStringLength, bytesRead
}

func nextLine(src []byte) (line, rest []byte) {
	for i, char := range src {
		if char == '\n' {
			return src[0 : i+1], src[i+1:]
		}
	}
	// do return partial lines please!
	return src, src[len(src):]
}

// Returns a maximum compressed size (in bytes) in worst case scenario. A buffer of this this size or greater is
// guaranteed to fit any result of Compress() call. Also a buffer of this size is guaranteed to fit any result of Decompress().
func DecompressBound() int {
	return MAX_CHUNK_SIZE + HEADER_SIZE
}

/*
Tries to unpack an integer number of compressed chunks.
dst - Buffer for output data. Should have at least DecompressBound() bytes size to guarantee there's enough space for any output.

	Smaller buffer of len(dst) = X may be used if it is known that at the time of compression input buffer B of len(B) <= X had been passed to Compress() function.

srcCompressed - Buffer with compressed input data. It should point at the beginning of a compressed chunk and should contain entire chunk or the function will fail and

	return with bytesRead == NOT_ENOUGH_INPUT.

Memory used by dst and srcCompressed must not overlap.

Two ints are returned:

  - bytesRead:      Number of bytes read from compressed buffer srcCompressed. Also may equal to one of three errors:

    -NOT_ENOUGH_INPUT:          srcCompressed did not contain one full chunk. Nothing was unpacked. Slice srcCompressed of greater Size is required to proceed.
    -NOT_ENOUGH_OUTPUT_SPACE:   dst was too small to store any unpacked chunks. Nothing was unpacked.
    -CORRUPT_INPUT:             srcCompressed does not contain a valid Logpack archive and cannot be unpacked.

  - bytesWritten:   Number of bytes written into output buffer Dst.
*/
func Decompress(dst, srcCompressed []byte) (bytesRead, bytesWritten int) {

	// buffer too small to contain even a header
	if len(srcCompressed) < HEADER_SIZE {
		return NOT_ENOUGH_INPUT, 0
	}
	chunkSize, rawSize := readHeader(srcCompressed)
	srcCompressed = srcCompressed[HEADER_SIZE:]

	// error - input buffer does not contain even one chunk (it's too small)
	if len(srcCompressed) < chunkSize {
		return NOT_ENOUGH_INPUT, 0
	}
	if len(dst) < rawSize {
		return NOT_ENOUGH_OUTPUT_SPACE, 0
	}

	bytesRead += chunkSize + HEADER_SIZE

	chunkResult := decompressChunk(srcCompressed[:chunkSize], dst[:rawSize])
	if chunkResult < 0 {
		return CORRUPT_INPUT, 0
	}

	bytesWritten += chunkResult

	srcCompressed = srcCompressed[chunkSize:]
	dst = dst[rawSize:]

	for len(srcCompressed) >= HEADER_SIZE {
		chunkSize, rawSize = readHeader(srcCompressed)
		srcCompressed = srcCompressed[HEADER_SIZE:]
		if len(srcCompressed) < chunkSize {
			return bytesRead, bytesWritten
		}
		if len(dst) < rawSize {
			return bytesRead, bytesWritten
		}

		bytesWritten += decompressChunk(srcCompressed[:chunkSize], dst[:rawSize])
		if chunkResult < 0 {
			return CORRUPT_INPUT, 0
		}

		srcCompressed = srcCompressed[chunkSize:]
		dst = dst[rawSize:]
		bytesRead += chunkSize + HEADER_SIZE
	}
	return bytesRead, bytesWritten
}

func decompressChunk(compressed, dst []byte) (bytesWritten int) {
	// fmt.Printf("DecompressChunk() len(compressed): %d; len(dst): %d\n", len(compressed), len(dst))
	backref := backrefBuffer{}
	backref.capacity = MAX_BACKREFERENCE_CAPACITY

	idxLineBegin := bytesWritten

	// Is compressed corrupt? If during packing, first byte of the chunk was > ESCAPE_FLAG,
	// it would have been prefixed/escaped with ESCAPE_FLAG;
	if compressed[0] > ESCAPE_BYTE {
		// fmt.Println("Decompress() failed! Line ref at the beginning of a chunk");
		return -1
	}

	// compressed is advanced one line per outer loop iteration; points to the first char of line
	for len(compressed) > 0 {
		var keyLine, lastDecompressedLine []byte
		idxKeyLine, idxCompressed := 0, 0

		// first char of line contains backreference to a line
		if compressed[idxCompressed] > ESCAPE_BYTE {
			firstByte := compressed[idxCompressed]
			compressed = compressed[1:]

			linesBefore := int(firstByte & ^(ESCAPE_BYTE | NO_SHARED_PREFIX_FLAG))
			keyLine = backref.getLineAt(linesBefore)

			if firstByte&NO_SHARED_PREFIX_FLAG != 0 {
				initialIdxKeyLine, bytesRead := decodeLength(compressed)
				idxKeyLine = initialIdxKeyLine
				compressed = compressed[bytesRead:]
			}
		}

		// For each char in line until newline plus
		// or stop on line bigger than chunk
		for idxCompressed < len(compressed) {
			// found encoded reference to string in keyLine
			if compressed[idxCompressed] > ESCAPE_BYTE {
				length, diffCompressed := decodeLength(compressed[idxCompressed:])
				idxCompressed += diffCompressed

				// this check triggers fail when encoded substring reference is longer than the actual referred line (which would cause OOB read)
				// it fails also in a situation where line reference references linesBefore that is not present in backrefBUffer - 
				// in such case backrefBuffer will return nil slice and len(nil) is 0 so this will always trigger
				if len(keyLine)-idxKeyLine < length {
					// fmt.Println("Decompress() failed! Reference too long for keyLine");
					return -1
				}

				copy(dst[bytesWritten:], keyLine[idxKeyLine:idxKeyLine+length])

				idxKeyLine = indexOfFirstSpace(idxKeyLine+length, keyLine)
				bytesWritten += length
				// LF reached, break to decompress next line
				if dst[bytesWritten-1] == '\n' {
					lastDecompressedLine = dst[idxLineBegin:bytesWritten]
					idxLineBegin = bytesWritten
					break
				}
			} else {
				// unquote and copy literally do dst
				if compressed[idxCompressed] == ESCAPE_BYTE {
					//skip ESCAPE_BYTE
					idxCompressed++
					if idxCompressed >= len(compressed) {
                        // fmt.Println("Decompress() failed! Unfinished escape sequence in input");
                        return -1;
                    }
				}

				if bytesWritten >= len(dst) {
                    // fmt.Println("Decompress() failed! Actual raw chunk size larger than declared in header");
                    return -1;
                }
				dst[bytesWritten] = compressed[idxCompressed]

				idxCompressed++
				bytesWritten++
				// LF reached, break to decompress next line
				if dst[bytesWritten-1] == '\n' {
					lastDecompressedLine = dst[idxLineBegin:bytesWritten]
					idxLineBegin = bytesWritten
					break
				}
			}
		}
		// fmt.Printf("Decompressed \"%s\"\n", lastDecompressedLine)
		backref.add(lastDecompressedLine)
		compressed = compressed[idxCompressed:]
	}
	return bytesWritten
}

func storeHeader(header []byte, compressedSize, rawSize int) {
	binary.LittleEndian.PutUint16(header, uint16(compressedSize-1))
	binary.LittleEndian.PutUint16(header[SIZEOF_INT16:], uint16(rawSize-1))
}

func readHeader(header []byte) (compressedSize, rawSize int) {
	return int(binary.LittleEndian.Uint16(header)) + 1,
		int(binary.LittleEndian.Uint16(header[SIZEOF_INT16:])) + 1
}

func Timer(name string) func() {
	start := time.Now()
	return func() {
		fmt.Printf("%s took %v micros\n", name, time.Since(start).Microseconds())
	}
}

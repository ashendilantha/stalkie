package matcher

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/bits"
	"net/http"
	"os"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

type ScaleFingerprint struct {
	H uint64 `json:"h"`
	V uint64 `json:"v"`
	D uint64 `json:"d"`
	A uint64 `json:"a"`
}

type ColorQuad struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

type ImageFingerprint struct {
	Full   ScaleFingerprint `json:"full"`
	Mid    ScaleFingerprint `json:"mid"`
	Close  ScaleFingerprint `json:"close"`
	Colors [4]ColorQuad     `json:"colors"`
}

func (f ImageFingerprint) IsZero() bool {
	return f.Full.H == 0 && f.Full.V == 0 && f.Full.D == 0 && f.Full.A == 0
}

func compareScales(s1, s2 ScaleFingerprint) int {
	dist := bits.OnesCount64(s1.H^s2.H) +
		bits.OnesCount64(s1.V^s2.V) +
		bits.OnesCount64(s1.D^s2.D) +
		bits.OnesCount64(s1.A^s2.A)
	score := int((256.0 - float64(dist)) / 256.0 * 100.0)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (f ImageFingerprint) Similarity(other ImageFingerprint) int {
	if f.IsZero() || other.IsZero() {
		return 0
	}

	scores := []int{
		compareScales(f.Full, other.Full),
		compareScales(f.Full, other.Mid),
		compareScales(f.Mid, other.Full),
		compareScales(f.Mid, other.Mid),
		compareScales(f.Mid, other.Close),
		compareScales(f.Close, other.Mid),
		compareScales(f.Close, other.Close),
	}

	maxGradScore := scores[0]
	for _, s := range scores[1:] {
		if s > maxGradScore {
			maxGradScore = s
		}
	}

	var totalColorDist float64
	for i := 0; i < 4; i++ {
		dr := float64(int(f.Colors[i].R) - int(other.Colors[i].R))
		dg := float64(int(f.Colors[i].G) - int(other.Colors[i].G))
		db := float64(int(f.Colors[i].B) - int(other.Colors[i].B))
		totalColorDist += math.Sqrt(dr*dr + dg*dg + db*db)
	}
	colorScore := int(100.0 - (totalColorDist / (4.0 * 441.67) * 100.0))
	if colorScore < 0 {
		colorScore = 0
	}
	if colorScore > 100 {
		colorScore = 100
	}

	if maxGradScore < 55 {
		maxGradScore = maxGradScore / 2
	}
	finalScore := int(float64(maxGradScore) * (0.3 + 0.7*(float64(colorScore)/100.0)))
	if colorScore < 45 {
		finalScore = int(float64(finalScore) * (float64(colorScore) / 50.0))
	}
	if finalScore < 0 {
		return 0
	}
	if finalScore > 100 {
		return 100
	}
	return finalScore
	return finalScore
}

func sampleBox(img image.Image, startX, startY, size int, scale *ScaleFingerprint) {
	sample := func(gx, gy, gridW, gridH int) uint32 {
		px := startX + (gx*size)/gridW
		py := startY + (gy*size)/gridH
		r, g, b, _ := img.At(px, py).RGBA()
		return (299*r + 587*g + 114*b) / 1000 >> 8
	}

	var aPixels [64]uint32
	var sum uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			val := sample(x, y, 8, 8)
			aPixels[y*8+x] = val
			sum += uint64(val)
		}
	}
	avg := uint32(sum / 64)
	for i := 0; i < 64; i++ {
		if aPixels[i] >= avg {
			scale.A |= 1 << i
		}
	}

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			left := sample(x, y, 9, 8)
			right := sample(x+1, y, 9, 8)
			if left > right {
				scale.H |= 1 << (y*8 + x)
			}
		}
	}

	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			top := sample(x, y, 8, 9)
			bottom := sample(x, y+1, 8, 9)
			if top > bottom {
				scale.V |= 1 << (x*8 + y)
			}
		}
	}

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			tl := sample(x, y, 9, 9)
			br := sample(x+1, y+1, 9, 9)
			if tl > br {
				scale.D |= 1 << (y*8 + x)
			}
		}
	}
}

func sampleColors(img image.Image, startX, startY, size int) [4]ColorQuad {
	var quads [4]ColorQuad
	half := size / 2
	if half <= 0 {
		return quads
	}

	quadOffsets := [4][2]int{
		{startX, startY},
		{startX + half, startY},
		{startX, startY + half},
		{startX + half, startY + half},
	}

	for q := 0; q < 4; q++ {
		qx := quadOffsets[q][0]
		qy := quadOffsets[q][1]
		var sumR, sumG, sumB uint32
		for sy := 0; sy < 4; sy++ {
			for sx := 0; sx < 4; sx++ {
				px := qx + (sx*half)/4
				py := qy + (sy*half)/4
				r, g, b, _ := img.At(px, py).RGBA()
				sumR += r >> 8
				sumG += g >> 8
				sumB += b >> 8
			}
		}
		quads[q] = ColorQuad{
			R: uint8(sumR / 16),
			G: uint8(sumG / 16),
			B: uint8(sumB / 16),
		}
	}
	return quads
}

func HashImage(img image.Image) ImageFingerprint {
	if img == nil {
		return ImageFingerprint{}
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return ImageFingerprint{}
	}

	dim := w
	if h < dim {
		dim = h
	}
	cx := bounds.Min.X + w/2
	cy := bounds.Min.Y + h/2

	sizeFull := dim
	startXFull := cx - sizeFull/2
	startYFull := cy - sizeFull/2

	sizeMid := int(float64(dim) * 0.82)
	if sizeMid <= 0 {
		sizeMid = dim
	}
	startXMid := cx - sizeMid/2
	startYMid := (cy - int(float64(dim)*0.04)) - sizeMid/2

	sizeClose := int(float64(dim) * 0.64)
	if sizeClose <= 0 {
		sizeClose = dim
	}
	startXClose := cx - sizeClose/2
	startYClose := (cy - int(float64(dim)*0.08)) - sizeClose/2

	var fp ImageFingerprint
	sampleBox(img, startXFull, startYFull, sizeFull, &fp.Full)
	sampleBox(img, startXMid, startYMid, sizeMid, &fp.Mid)
	sampleBox(img, startXClose, startYClose, sizeClose, &fp.Close)
	fp.Colors = sampleColors(img, startXFull, startYFull, sizeFull)

	return fp
}

func HashReader(r io.Reader) (ImageFingerprint, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return ImageFingerprint{}, err
	}
	return HashImage(img), nil
}

func HashFile(path string) (ImageFingerprint, error) {
	f, err := os.Open(path)
	if err != nil {
		return ImageFingerprint{}, err
	}
	defer f.Close()
	return HashReader(f)
}

func (f ImageFingerprint) FetchAndCompare(client *http.Client, avatarURL string, timeout time.Duration, minSim int) (int, bool) {
	if f.IsZero() || avatarURL == "" {
		return 0, false
	}

	req, err := http.NewRequest(http.MethodGet, avatarURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

	fetchClient := client
	if timeout > 0 {
		fetchClient = &http.Client{
			Transport: client.Transport,
			Timeout:   timeout,
		}
	}

	resp, err := fetchClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return 0, false
	}

	limited := io.LimitReader(resp.Body, 8*1024*1024)
	remoteFp, err := HashReader(limited)
	if err != nil {
		return 0, false
	}

	score := f.Similarity(remoteFp)
	matched := score >= minSim
	return score, matched
}

func ExpandName(fullName string) []string {
	clean := strings.TrimSpace(fullName)
	if clean == "" {
		return nil
	}

	words := strings.Fields(strings.ToLower(clean))
	if len(words) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var candidates []string

	add := func(c string) {
		trimmed := strings.TrimSpace(strings.ToLower(c))
		if trimmed == "" || len(trimmed) < 2 {
			return
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			candidates = append(candidates, trimmed)
		}
	}

	if len(words) == 1 {
		w := words[0]
		add(w)
		add(w + "_")
		add(w + "-dev")
		add(w + "_dev")
		add("real" + w)
		add("the" + w)
		add(w + "01")
		add(w + "1")
		add("iam" + w)
		return candidates
	}

	first := words[0]
	last := words[len(words)-1]

	add(first + last)
	add(last + first)
	add(first + "_" + last)
	add(first + "." + last)
	add(first + "-" + last)
	add(last + "_" + first)
	add(last + "." + first)

	if len(first) > 0 {
		add(string(first[0]) + last)
		add(string(first[0]) + "_" + last)
		add(string(first[0]) + "." + last)
		add(string(first[0]) + "-" + last)
	}

	if len(last) > 0 {
		add(first + string(last[0]))
		add(first + "_" + string(last[0]))
		add(first + "." + string(last[0]))
		add(first + "-" + string(last[0]))
	}

	if len(words) > 2 {
		var initials string
		for _, w := range words[:len(words)-1] {
			if len(w) > 0 {
				initials += string(w[0])
			}
		}
		add(initials + last)
		add(initials + "_" + last)
		add(strings.Join(words, ""))
		add(strings.Join(words, "_"))
		add(strings.Join(words, "."))
	}

	add(first)
	add(last)

	suffixes := []string{"_", "-dev", "01", "1", "_official", "dev"}
	prefixes := []string{"real", "the", "iam"}

	primary := first + last
	for _, sfx := range suffixes {
		add(primary + sfx)
		add(first + sfx)
	}
	for _, pfx := range prefixes {
		add(pfx + primary)
		add(pfx + first)
	}

	return candidates
}

package rtp

const (
	// NUMBER_OF_BUTTONS represents the total capacity for detected DTMF tones.
	NUMBER_OF_BUTTONS = 65
	// NUMBER_BUTTONS_GEN represents the total capacity for generated DTMF tones.
	NUMBER_BUTTONS_GEN = 20
)

// DTMFDetector is used for detecting DTMF frequencies.
type DTMFDetector struct {
	dialToneCount       int
	pDialButtons        []rune
	indexForDialButtons int

	COEFF_NUMBER   int
	CONSTANTS      []int16
	pArraySamples  []int16
	T              []int
	internalArray  []int16
	frameSize      int
	SAMPLES        int
	frame_count    int
	prevDialButton rune
	permissionFlag bool

	powerThreshold            int
	dialTonesToOhersTones     int
	dialTonesToOhersDialTones int
}

// NewDTMFDetector initializes a DTMFDetector with a given capacity for DTMF tones.
func NewDTMFDetector(dtmfCount int) *DTMFDetector {
	if dtmfCount <= 0 {
		dtmfCount = NUMBER_OF_BUTTONS
	}
	return &DTMFDetector{
		dialToneCount:       dtmfCount,
		pDialButtons:        make([]rune, dtmfCount),
		indexForDialButtons: 0,

		COEFF_NUMBER:   18,
		CONSTANTS:      []int16{27860, 26745, 25529, 24216, 19747, 16384, 12773, 8967, 21319, 29769, 32706, 32210, 31778, 31226, -1009, -12772, -22811, -30555},
		pArraySamples:  nil,
		T:              make([]int, 18),
		internalArray:  make([]int16, 102),
		frameSize:      0,
		SAMPLES:        102,
		frame_count:    0,
		prevDialButton: ' ',
		permissionFlag: false,

		powerThreshold:            328,
		dialTonesToOhersTones:     16,
		dialTonesToOhersDialTones: 6,
	}
}

// norm_l calculates a normalized value for the given input.
func (d *DTMFDetector) norm_l(L_var1 int) int16 {
	var var_out int16

	if L_var1 == 0 {
		var_out = 0
	} else {
		if L_var1 == -1 {
			var_out = 31
		} else {
			if L_var1 < 0 {
				L_var1 = ^L_var1
			}

			for var_out = 0; L_var1 < 0x40000000; var_out++ {
				L_var1 <<= 1
			}
		}
	}
	return var_out
}

// dtmfDetection performs DTMF detection on the provided audio samples.
func (d *DTMFDetector) dtmfDetection(short_array_samples []int16) rune {
	Dial := int16(32)
	var Sum int

	for ii := 0; ii < d.SAMPLES; ii++ {
		if short_array_samples[ii] >= 0 {
			Sum += int(short_array_samples[ii])
		} else {
			Sum -= int(short_array_samples[ii])
		}
	}

	Sum /= d.SAMPLES
	if Sum < d.powerThreshold {
		return ' '
	}

	for ii := 0; ii < d.SAMPLES; ii++ {
		d.T[0] = int(short_array_samples[ii])
		if d.T[0] != 0 {
			if Dial > d.norm_l(d.T[0]) {
				Dial = d.norm_l(d.T[0])
			}
		}
	}

	Dial -= 16

	for ii := 0; ii < d.SAMPLES; ii++ {
		d.T[0] = int(short_array_samples[ii])
		d.internalArray[ii] = int16(d.T[0]) << Dial
	}

	d.goertzelFilter(d.CONSTANTS[0], d.CONSTANTS[1], d.internalArray, d.T, d.SAMPLES, 0)
	d.goertzelFilter(d.CONSTANTS[2], d.CONSTANTS[3], d.internalArray, d.T, d.SAMPLES, 2)
	d.goertzelFilter(d.CONSTANTS[4], d.CONSTANTS[5], d.internalArray, d.T, d.SAMPLES, 4)
	d.goertzelFilter(d.CONSTANTS[6], d.CONSTANTS[7], d.internalArray, d.T, d.SAMPLES, 6)
	d.goertzelFilter(d.CONSTANTS[8], d.CONSTANTS[9], d.internalArray, d.T, d.SAMPLES, 8)
	d.goertzelFilter(d.CONSTANTS[10], d.CONSTANTS[11], d.internalArray, d.T, d.SAMPLES, 10)
	d.goertzelFilter(d.CONSTANTS[12], d.CONSTANTS[13], d.internalArray, d.T, d.SAMPLES, 12)
	d.goertzelFilter(d.CONSTANTS[14], d.CONSTANTS[15], d.internalArray, d.T, d.SAMPLES, 14)
	d.goertzelFilter(d.CONSTANTS[16], d.CONSTANTS[17], d.internalArray, d.T, d.SAMPLES, 16)

	Row := 0
	Temp := 0

	for ii := 0; ii < 4; ii++ {
		if Temp < d.T[ii] {
			Row = ii
			Temp = d.T[ii]
		}
	}

	Column := 4
	Temp = 0

	for ii := 4; ii < 8; ii++ {
		if Temp < d.T[ii] {
			Column = ii
			Temp = d.T[ii]
		}
	}

	Sum = 0

	for ii := 0; ii < 10; ii++ {
		Sum += d.T[ii]
	}
	Sum -= d.T[Row]
	Sum -= d.T[Column]
	Sum >>= 3

	if Sum == 0 {
		Sum = 1
	}

	if d.T[Row]/Sum < d.dialTonesToOhersDialTones {
		return ' '
	}
	if d.T[Column]/Sum < d.dialTonesToOhersDialTones {
		return ' '
	}

	if d.T[Row] < (d.T[Column] >> 2) {
		return ' '
	}

	if d.T[Column] < ((d.T[Row] >> 1) - (d.T[Row] >> 3)) {
		return ' '
	}

	for ii := 0; ii < d.COEFF_NUMBER; ii++ {
		if d.T[ii] == 0 {
			d.T[ii] = 1
		}
	}

	for ii := 10; ii < d.COEFF_NUMBER; ii++ {
		if d.T[Row]/d.T[ii] < d.dialTonesToOhersTones {
			return ' '
		}
		if d.T[Column]/d.T[ii] < d.dialTonesToOhersTones {
			return ' '
		}
	}

	for ii := 0; ii < 10; ii++ {
		if d.T[ii] != d.T[Column] {
			if d.T[ii] != d.T[Row] {
				if d.T[Row]/d.T[ii] < d.dialTonesToOhersDialTones {
					return ' '
				}
				if Column != 4 {
					if d.T[Column]/d.T[ii] < d.dialTonesToOhersDialTones {
						return ' '
					}
				} else {
					if d.T[Column]/d.T[ii] < (d.dialTonesToOhersDialTones / 3) {
						return ' '
					}
				}
			}
		}
	}

	var return_value rune
	switch Row {
	case 0:
		switch Column {
		case 4:
			return_value = '1'
		case 5:
			return_value = '2'
		case 6:
			return_value = '3'
		case 7:
			return_value = 'A'
		}
	case 1:
		switch Column {
		case 4:
			return_value = '4'
		case 5:
			return_value = '5'
		case 6:
			return_value = '6'
		case 7:
			return_value = 'B'
		}
	case 2:
		switch Column {
		case 4:
			return_value = '7'
		case 5:
			return_value = '8'
		case 6:
			return_value = '9'
		case 7:
			return_value = 'C'
		}
	case 3:
		switch Column {
		case 4:
			return_value = '*'
		case 5:
			return_value = '0'
		case 6:
			return_value = '#'
		case 7:
			return_value = 'D'
		}
	}

	return return_value
}

// GetDetectedDTMFCount returns the number of detected DTMF tones.
func (d *DTMFDetector) GetDetectedDTMFCount() int {
	return d.indexForDialButtons
}

// GetAllDTMFTones returns all detected DTMF tones as a slice of runes.
func (d *DTMFDetector) GetAllDTMFTones() []rune {
	return d.pDialButtons
}

// GetDTMFTones returns the detected DTMF tones, excluding null characters.
func (d *DTMFDetector) GetDTMFTones() []rune {
	result := []rune{}
	for _, r := range d.pDialButtons {
		if r != '\x00' {
			result = append(result, r)
		}
	}
	return result
}

// GetDetectedDTMFTones returns the detected DTMF tones as a slice of strings.
func (d *DTMFDetector) GetDetectedDTMFTones() []string {
	result := []string{}
	for _, r := range d.pDialButtons {
		if r != '\x00' {
			result = append(result, string(r))
		}
	}
	return result
}

// GetFirstDetectedDTMFTone returns the first detected DTMF tone as a string.
func (d *DTMFDetector) GetFirstDetectedDTMFTone() string {
	for _, r := range d.pDialButtons {
		if r != '\x00' {
			return string(r)
		}
	}
	return ""
}

// zerosIndexDialButtons resets the index for dial buttons, preparing for the next set of detections.
func (d *DTMFDetector) zerosIndexDialButtons() {
	d.indexForDialButtons = 0
}

// reset reinitializes the detector with a given frame size.
func (d *DTMFDetector) reset(frameSize_ int) {
	d.frameSize = frameSize_
	d.pArraySamples = make([]int16, d.frameSize+d.SAMPLES)
	d.pDialButtons[0] = '\x00'
	d.frame_count = 0
	d.prevDialButton = ' '
	d.permissionFlag = false
	d.indexForDialButtons = 0
}

// Detect performs DTMF detection on the input frame of audio samples.
func (d *DTMFDetector) Detect(input_frame []int16) {
	d.reset(len(input_frame))

	var ii int
	var temp_dial_button rune

	for ii = 0; ii < d.frameSize; ii++ {
		d.pArraySamples[ii+d.frame_count] = input_frame[ii]
	}

	d.frame_count += d.frameSize
	temp_index := 0
	if d.frame_count >= d.SAMPLES {
		for d.frame_count >= d.SAMPLES {
			if temp_index == 0 {
				temp_dial_button = d.dtmfDetection(d.pArraySamples)
			} else {
				tempArray := make([]int16, len(d.pArraySamples)-temp_index)
				for inc := 0; inc < len(d.pArraySamples)-temp_index; inc++ {
					tempArray[inc] = d.pArraySamples[temp_index+inc]
				}
				temp_dial_button = d.dtmfDetection(tempArray)
			}

			if d.permissionFlag {
				if temp_dial_button != ' ' {
					d.pDialButtons[d.indexForDialButtons] = temp_dial_button
					d.indexForDialButtons++
					d.pDialButtons[d.indexForDialButtons] = '\x00'
					if d.indexForDialButtons >= d.dialToneCount {
						d.indexForDialButtons = 0
					}
				}
				d.permissionFlag = false
			}

			if (temp_dial_button != ' ') && (d.prevDialButton == ' ') {
				d.permissionFlag = true
			}

			d.prevDialButton = temp_dial_button

			temp_index += d.SAMPLES
			d.frame_count -= d.SAMPLES
		}

		for ii = 0; ii < d.frame_count; ii++ {
			d.pArraySamples[ii] = d.pArraySamples[ii+temp_index]
		}
	}
}

// mpy48sr performs a multiplication operation on the input values.
func (d *DTMFDetector) mpy48sr(o16 int16, o32 int) int {
	var Temp0 int
	var Temp1 int
	Temp0 = (((int(o32) * int(o16)) + 0x4000) >> 15)
	Temp1 = (int(o32) >> 16) * int(o16)
	return (int)((Temp1 << 1) + Temp0)
}

// goertzelFilter applies the Goertzel filter to the input samples.
func (d *DTMFDetector) goertzelFilter(Koeff0 int16, Koeff1 int16, arraySamples []int16, Magnitude []int, COUNT int, index int) {
	var Temp0, Temp1 int
	var Vk1_0, Vk2_0, Vk1_1, Vk2_1 int

	for ii := 0; ii < COUNT; ii++ {
		Temp0 = d.mpy48sr(Koeff0, Vk1_0<<1) - Vk2_0 + int(arraySamples[ii])
		Temp1 = d.mpy48sr(Koeff1, Vk1_1<<1) - Vk2_1 + int(arraySamples[ii])

		Vk2_0 = Vk1_0
		Vk2_1 = Vk1_1
		Vk1_0 = Temp0
		Vk1_1 = Temp1
	}

	Vk1_0 >>= 10
	Vk1_1 >>= 10
	Vk2_0 >>= 10
	Vk2_1 >>= 10
	Temp0 = d.mpy48sr(Koeff0, Vk1_0<<1)
	Temp1 = d.mpy48sr(Koeff1, Vk1_1<<1)
	Temp0 = int(int16(Temp0)) * int(int16(Vk2_0))
	Temp1 = int(int16(Temp1)) * int(int16(Vk2_1))
	Temp0 = int(int16(Vk1_0))*int(int16(Vk1_0)) + int(int16(Vk2_0))*int(int16(Vk2_0)) - Temp0
	Temp1 = int(int16(Vk1_1))*int(int16(Vk1_1)) + int(int16(Vk2_1))*int(int16(Vk2_1)) - Temp1
	Magnitude[index] = Temp0
	Magnitude[index+1] = Temp1
	return
}

// DTMFGenerator generates DTMF frequencies for given buttons.
type DTMFGenerator struct {
	countDurationPushButton     int
	countDurationPause          int
	tempCountDurationPushButton int
	tempCountDurationPause      int
	readyFlag                   bool
	pushDialButtons             []rune
	countLengthDialButtonsArray int
	count                       int
	sizeOfFrame                 int
	tempCoeff                   []int16
	tempCoeff1                  int16
	tempCoeff2                  int16
	y1_1                        int
	y1_2                        int
	y2_1                        int
	y2_2                        int
}

// NewDTMFGenerator initializes a DTMFGenerator with frame size, tone duration, and pause duration.
func NewDTMFGenerator(frameSize int, durationPush int, durationPause int) *DTMFGenerator {
	return &DTMFGenerator{
		countDurationPushButton:     (durationPush<<3)/frameSize + 1,
		countDurationPause:          (durationPause<<3)/frameSize + 1,
		sizeOfFrame:                 frameSize,
		readyFlag:                   true,
		countLengthDialButtonsArray: 0,
		pushDialButtons:             make([]rune, NUMBER_BUTTONS_GEN),
		tempCoeff:                   []int16{27980, 26956, 25701, 24218, 19073, 16325, 13085, 9315},
	}
}

// dtmfGenerating generates DTMF tones for the provided buttons and writes them to the output buffer.
func (g *DTMFGenerator) dtmfGenerating(y []int16) {
	if g.readyFlag {
		return
	}

	for g.countLengthDialButtonsArray > 0 {
		if g.countDurationPushButton == g.tempCountDurationPushButton {
			switch g.pushDialButtons[g.count] {
			case '1':
				g.tempCoeff1 = g.tempCoeff[0]
				g.tempCoeff2 = g.tempCoeff[4]
				g.y1_1 = int(g.tempCoeff[0])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[4])
				g.y2_2 = 31000
			case '2':
				g.tempCoeff1 = g.tempCoeff[0]
				g.tempCoeff2 = g.tempCoeff[5]
				g.y1_1 = int(g.tempCoeff[0])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[5])
				g.y2_2 = 31000
			case '3':
				g.tempCoeff1 = g.tempCoeff[0]
				g.tempCoeff2 = g.tempCoeff[6]
				g.y1_1 = int(g.tempCoeff[0])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[6])
				g.y2_2 = 31000
			case 'A':
				g.tempCoeff1 = g.tempCoeff[0]
				g.tempCoeff2 = g.tempCoeff[7]
				g.y1_1 = int(g.tempCoeff[0])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[7])
				g.y2_2 = 31000
			case '4':
				g.tempCoeff1 = g.tempCoeff[1]
				g.tempCoeff2 = g.tempCoeff[4]
				g.y1_1 = int(g.tempCoeff[1])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[4])
				g.y2_2 = 31000
			case '5':
				g.tempCoeff1 = g.tempCoeff[1]
				g.tempCoeff2 = g.tempCoeff[5]
				g.y1_1 = int(g.tempCoeff[1])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[5])
				g.y2_2 = 31000
			case '6':
				g.tempCoeff1 = g.tempCoeff[1]
				g.tempCoeff2 = g.tempCoeff[6]
				g.y1_1 = int(g.tempCoeff[1])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[6])
				g.y2_2 = 31000
			case 'B':
				g.tempCoeff1 = g.tempCoeff[1]
				g.tempCoeff2 = g.tempCoeff[7]
				g.y1_1 = int(g.tempCoeff[1])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[7])
				g.y2_2 = 31000
			case '7':
				g.tempCoeff1 = g.tempCoeff[2]
				g.tempCoeff2 = g.tempCoeff[4]
				g.y1_1 = int(g.tempCoeff[2])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[4])
				g.y2_2 = 31000
			case '8':
				g.tempCoeff1 = g.tempCoeff[2]
				g.tempCoeff2 = g.tempCoeff[5]
				g.y1_1 = int(g.tempCoeff[2])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[5])
				g.y2_2 = 31000
			case '9':
				g.tempCoeff1 = g.tempCoeff[2]
				g.tempCoeff2 = g.tempCoeff[6]
				g.y1_1 = int(g.tempCoeff[2])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[6])
				g.y2_2 = 31000
			case 'C':
				g.tempCoeff1 = g.tempCoeff[2]
				g.tempCoeff2 = g.tempCoeff[7]
				g.y1_1 = int(g.tempCoeff[2])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[7])
				g.y2_2 = 31000
			case '*':
				g.tempCoeff1 = g.tempCoeff[3]
				g.tempCoeff2 = g.tempCoeff[4]
				g.y1_1 = int(g.tempCoeff[3])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[4])
				g.y2_2 = 31000
			case '0':
				g.tempCoeff1 = g.tempCoeff[3]
				g.tempCoeff2 = g.tempCoeff[5]
				g.y1_1 = int(g.tempCoeff[3])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[5])
				g.y2_2 = 31000
			case '#':
				g.tempCoeff1 = g.tempCoeff[3]
				g.tempCoeff2 = g.tempCoeff[6]
				g.y1_1 = int(g.tempCoeff[3])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[6])
				g.y2_2 = 31000
			case 'D':
				g.tempCoeff1 = g.tempCoeff[3]
				g.tempCoeff2 = g.tempCoeff[7]
				g.y1_1 = int(g.tempCoeff[3])
				g.y2_1 = 31000
				g.y1_2 = int(g.tempCoeff[7])
				g.y2_2 = 31000
			default:
				g.tempCoeff1 = 0
				g.tempCoeff2 = 0
				g.y1_1 = 0
				g.y2_1 = 0
				g.y1_2 = 0
				g.y2_2 = 0
			}
		}

		for g.tempCountDurationPushButton > 0 {
			g.tempCountDurationPushButton--
			g.frequencyOscillator(g.tempCoeff1, g.tempCoeff2, y, g.sizeOfFrame)
			return
		}

		for g.tempCountDurationPause > 0 {
			g.tempCountDurationPause--
			for ii := 0; ii < g.sizeOfFrame; ii++ {
				y[ii] = 0
			}
			return
		}

		g.tempCountDurationPushButton = g.countDurationPushButton
		g.tempCountDurationPause = g.countDurationPause

		g.count++
		g.countLengthDialButtonsArray--
	}
	g.readyFlag = true
	return
}

// transmitNewDialButtonsArray transmits a new array of dial buttons for generation.
func (g *DTMFGenerator) transmitNewDialButtonsArray(dialButtonsArray []rune, lengthDialButtonsArray int) int {
	if !g.getReadyFlag() {
		return 0
	}
	if lengthDialButtonsArray == 0 {
		g.countLengthDialButtonsArray = 0
		g.count = 0
		g.readyFlag = true
		return 1
	}
	g.countLengthDialButtonsArray = lengthDialButtonsArray
	if lengthDialButtonsArray > NUMBER_BUTTONS_GEN {
		g.countLengthDialButtonsArray = NUMBER_BUTTONS_GEN
	}
	copy(g.pushDialButtons, dialButtonsArray[:g.countLengthDialButtonsArray])

	g.tempCountDurationPushButton = g.countDurationPushButton
	g.tempCountDurationPause = g.countDurationPause

	g.count = 0
	g.readyFlag = false
	return 1
}

// dtmfGeneratorReset resets the generator to its initial state.
func (g *DTMFGenerator) dtmfGeneratorReset() {
	g.countLengthDialButtonsArray = 0
	g.count = 0
	g.readyFlag = true
}

// getReadyFlag returns true if the generator is ready to accept new buttons, false otherwise.
func (g *DTMFGenerator) getReadyFlag() bool {
	return g.readyFlag
}

// mpy48sr performs a multiplication operation on the input values.
func (g *DTMFGenerator) mpy48sr(o16 int16, o32 int) int {
	var Temp0 int
	var Temp1 int
	Temp0 = (((int(o32) * int(o16)) + 0x4000) >> 15)
	Temp1 = (int(o32) >> 16) * int(o16)
	return (int)((Temp1 << 1) + Temp0)
}

// frequencyOscillator generates the frequency for the given coefficients and writes it to the output buffer.
func (g *DTMFGenerator) frequencyOscillator(Coeff0 int16, Coeff1 int16, y []int16, COUNT int) {
	var Temp1_0, Temp1_1, Temp2_0, Temp2_1, Temp0, Temp1, Subject int

	Temp1_0 = g.y1_1
	Temp1_1 = g.y1_2
	Temp2_0 = g.y2_1
	Temp2_1 = g.y2_2
	Subject = int(Coeff0) * int(Coeff1)

	for ii := 0; ii < COUNT; ii++ {
		Temp0 = g.mpy48sr(Coeff0, Temp1_0<<1) - Temp2_0
		Temp1 = g.mpy48sr(Coeff1, Temp1_1<<1) - Temp2_1
		Temp2_0 = Temp1_0
		Temp2_1 = Temp1_1
		Temp1_0 = Temp0
		Temp1_1 = Temp1
		Temp0 += Temp1
		if Subject != 0 {
			Temp0 >>= 1
		}
		y[ii] = int16(Temp0)
	}

	g.y1_1 = Temp1_0
	g.y1_2 = Temp1_1
	g.y2_1 = Temp2_0
	g.y2_2 = Temp2_1
}

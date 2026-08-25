package service

const (
	muLawBias = 0x84
	muLawClip = 32635
)

func linear16SampleToMulaw(sample int16) byte {
	sign := byte(0)
	s := int(sample)

	if s < 0 {
		sign = 0x80
		s = -s
	}

	if s > muLawClip {
		s = muLawClip
	}

	s += muLawBias

	exponent := byte(7)
	expMask := 0x4000

	for expMask > 0 && (s&expMask) == 0 {
		exponent--
		expMask >>= 1
	}

	mantissa := byte((s >> (uint(exponent) + 3)) & 0x0F)

	return ^(sign | (exponent << 4) | mantissa)
}

func Downsample24kLinear16ToMulaw8k(pcm []byte) []byte {
	if len(pcm) < 6 {
		return nil
	}

	sampleCount := len(pcm) / 2
	frameCount := sampleCount / 3

	out := make([]byte, 0, frameCount)

	for i := 0; i+6 <= len(pcm); i += 6 {
		s1 := int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8)
		s2 := int16(uint16(pcm[i+2]) | uint16(pcm[i+3])<<8)
		s3 := int16(uint16(pcm[i+4]) | uint16(pcm[i+5])<<8)

		avg := (int32(s1) + int32(s2) + int32(s3)) / 3

		out = append(out, linear16SampleToMulaw(int16(avg)))
	}

	return out
}

package service

import "sync"

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

type PCMDownsampler struct {
	mu       sync.Mutex
	leftover []byte
}

func NewPCMDownsampler() *PCMDownsampler {
	return &PCMDownsampler{}
}

func (d *PCMDownsampler) Push(pcm []byte) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	buf := append(d.leftover, pcm...)
	usable := len(buf) - (len(buf) % 6)

	out := make([]byte, 0, usable/6)

	for i := 0; i+6 <= usable; i += 6 {
		s1 := int16(uint16(buf[i]) | uint16(buf[i+1])<<8)
		s2 := int16(uint16(buf[i+2]) | uint16(buf[i+3])<<8)
		s3 := int16(uint16(buf[i+4]) | uint16(buf[i+5])<<8)

		avg := (int32(s1) + int32(s2) + int32(s3)) / 3

		out = append(out, linear16SampleToMulaw(int16(avg)))
	}

	d.leftover = append([]byte(nil), buf[usable:]...)

	return out
}

func (d *PCMDownsampler) Reset() {
	d.mu.Lock()
	d.leftover = nil
	d.mu.Unlock()
}

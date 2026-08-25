package service

import "sync"

const (
	muLawBias = 0x84
	muLawClip = 32635
)

func linear16SampleToMulaw(sample int16) byte {
	pcm := int(sample)
	sign := byte(0)

	if pcm < 0 {
		sign = 0x80
		pcm = -pcm
	}

	if pcm > muLawClip {
		pcm = muLawClip
	}

	pcm += muLawBias

	exponent := byte(7)

	for expMask := 0x4000; exponent > 0; expMask >>= 1 {
		if pcm&expMask != 0 {
			break
		}

		exponent--
	}

	mantissa := byte((pcm >> (uint(exponent) + 3)) & 0x0F)

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

	if len(pcm) == 0 && len(d.leftover) == 0 {
		return nil
	}

	buf := make(
		[]byte,
		0,
		len(d.leftover)+len(pcm),
	)

	buf = append(buf, d.leftover...)
	buf = append(buf, pcm...)

	usable := len(buf) - (len(buf) % 6)

	if usable == 0 {
		d.leftover = append(
			d.leftover[:0],
			buf...,
		)

		return nil
	}

	out := make(
		[]byte,
		0,
		usable/6,
	)

	for i := 0; i < usable; i += 6 {
		s1 := int16(
			uint16(buf[i]) |
				uint16(buf[i+1])<<8,
		)

		s2 := int16(
			uint16(buf[i+2]) |
				uint16(buf[i+3])<<8,
		)

		s3 := int16(
			uint16(buf[i+4]) |
				uint16(buf[i+5])<<8,
		)

		avg := int16(
			(int32(s1) +
				int32(s2) +
				int32(s3)) / 3,
		)

		out = append(
			out,
			linear16SampleToMulaw(avg),
		)
	}

	d.leftover = append(
		d.leftover[:0],
		buf[usable:]...,
	)

	return out
}

func (d *PCMDownsampler) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.leftover = nil
}

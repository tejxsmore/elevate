package service

import "sync"

const (
	muLawBias = 0x84
	muLawClip = 32635
)

func linear16SampleToMulaw(sample int16) byte {
	s := int(sample)
	sign := byte(0)

	if s < 0 {
		sign = 0x80
		s = -s
	}

	if s > muLawClip {
		s = muLawClip
	}

	s += muLawBias

	exponent := 7
	expMask := 0x4000

	for exponent > 0 && (s&expMask) == 0 {
		exponent--
		expMask >>= 1
	}

	mantissa := (s >> (exponent + 3)) & 0x0F

	return ^(sign | byte(exponent<<4) | byte(mantissa))
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

	buf = append(
		buf,
		d.leftover...,
	)

	buf = append(
		buf,
		pcm...,
	)

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

		filtered := int32(s1) +
			2*int32(s2) +
			int32(s3)

		filtered /= 4

		if filtered > 32767 {
			filtered = 32767
		}

		if filtered < -32768 {
			filtered = -32768
		}

		out = append(
			out,
			linear16SampleToMulaw(
				int16(filtered),
			),
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

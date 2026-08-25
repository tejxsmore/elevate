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

	mantissa := byte(
		(pcm >> (uint(exponent) + 3)) & 0x0F,
	)

	return ^(sign | (exponent << 4) | mantissa)
}

type PCMDownsampler struct {
	mu       sync.Mutex
	leftover []byte
	samples  []int16
}

func NewPCMDownsampler() *PCMDownsampler {
	return &PCMDownsampler{
		samples: make(
			[]int16,
			0,
			4096,
		),
	}
}

func (d *PCMDownsampler) Push(
	pcm []byte,
) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(pcm) == 0 &&
		len(d.leftover) == 0 {
		return nil
	}

	data := make(
		[]byte,
		0,
		len(d.leftover)+len(pcm),
	)

	data = append(
		data,
		d.leftover...,
	)

	data = append(
		data,
		pcm...,
	)

	usableBytes := len(data)

	if usableBytes%2 != 0 {
		usableBytes--
	}

	d.leftover = append(
		d.leftover[:0],
		data[usableBytes:]...,
	)

	for i := 0; i < usableBytes; i += 2 {
		sample := int16(
			uint16(data[i]) |
				uint16(data[i+1])<<8,
		)

		d.samples = append(
			d.samples,
			sample,
		)
	}

	const samplesPerOutput = 3

	usableSamples :=
		len(d.samples) -
			(len(d.samples) % samplesPerOutput)

	if usableSamples == 0 {
		return nil
	}

	out := make(
		[]byte,
		0,
		usableSamples/samplesPerOutput,
	)

	for i := 0; i < usableSamples; i += samplesPerOutput {
		sum := int32(d.samples[i]) +
			int32(d.samples[i+1]) +
			int32(d.samples[i+2])

		sample := int16(
			sum / samplesPerOutput,
		)

		out = append(
			out,
			linear16SampleToMulaw(sample),
		)
	}

	copy(
		d.samples,
		d.samples[usableSamples:],
	)

	d.samples =
		d.samples[:len(d.samples)-usableSamples]

	return out
}

func (d *PCMDownsampler) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.leftover = nil
	d.samples = d.samples[:0]
}

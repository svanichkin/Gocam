//go:build linux
// +build linux

package gocam

import (
	"context"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	vidiocQuerycap  = 0x80685600
	vidiocSFmt      = 0xC0D05605
	vidiocReqbufs   = 0xC0145608
	vidiocQuerybuf  = 0xC0445609
	vidiocQBuf      = 0xC044560F
	vidiocDQBuf     = 0xC0445611
	vidiocStreamOn  = 0x40045612
	vidiocStreamOff = 0x40045613

	v4l2BufTypeVideoCapture = 1
	v4l2FieldAny            = 0
	v4l2MemoryMMap          = 1

	v4l2PixFmtRGB24 = 0x33424752 // 'RGB3'
	v4l2PixFmtYUYV  = 0x56595559 // 'YUYV'

	v4l2CapVideoCapture = 0x00000001
	v4l2CapStreaming    = 0x04000000
	v4l2CapDeviceCaps   = 0x80000000
)

type v4l2Capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

type v4l2PixFormat struct {
	Width        uint32
	Height       uint32
	Pixelformat  uint32
	Field        uint32
	Bytesperline uint32
	Sizeimage    uint32
	Colorspace   uint32
	Priv         uint32
	Flags        uint32
	YcbcrEnc     uint32
	Quantization uint32
	XferFunc     uint32
}

type v4l2Format struct {
	Type uint32
	fmt  [200]byte
}

type v4l2RequestBuffers struct {
	Count    uint32
	Type     uint32
	Memory   uint32
	Reserved [2]uint32
}

type v4l2Timecode struct {
	Type     uint32
	Flags    uint32
	Frames   uint8
	Seconds  uint8
	Minutes  uint8
	Hours    uint8
	Userbits [4]uint8
}

type v4l2Buffer struct {
	Index     uint32
	Type      uint32
	Bytesused uint32
	Flags     uint32
	Field     uint32
	Timestamp syscall.Timeval
	Timecode  v4l2Timecode
	Sequence  uint32
	Memory    uint32
	Offset    uint32
	_         uint32 // union padding
	Length    uint32
	Reserved2 uint32
	Reserved  uint32
}

type mappedBuffer struct {
	data   []byte
	length uint32
}

func StartStream(ctx context.Context) (<-chan Frame, error) {
	fd, err := syscall.Open("/dev/video0", syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("gocam: cannot open /dev/video0: %w", err)
	}

	var (
		buffers       []mappedBuffer
		streamStarted bool
	)

	cleanup := func() {
		if streamStarted {
			bufType := uint32(v4l2BufTypeVideoCapture)
			_ = ioctl(fd, vidiocStreamOff, unsafe.Pointer(&bufType))
		}
		for _, mb := range buffers {
			if mb.data != nil {
				_ = syscall.Munmap(mb.data)
			}
		}
		_ = syscall.Close(fd)
	}

	var caps v4l2Capability
	if err := ioctl(fd, vidiocQuerycap, unsafe.Pointer(&caps)); err != nil {
		cleanup()
		return nil, fmt.Errorf("gocam: VIDIOC_QUERYCAP failed: %w", err)
	}

	capsToCheck := caps.Capabilities
	if capsToCheck&v4l2CapDeviceCaps != 0 {
		capsToCheck = caps.DeviceCaps
	}
	if capsToCheck&v4l2CapVideoCapture == 0 {
		cleanup()
		return nil, fmt.Errorf("gocam: device does not support video capture")
	}
	if capsToCheck&v4l2CapStreaming == 0 {
		cleanup()
		return nil, fmt.Errorf("gocam: device does not support streaming I/O")
	}

	const (
		defaultWidth  = 352
		defaultHeight = 288
	)

	width := uint32(defaultWidth)
	height := uint32(defaultHeight)
	pixelFormat := uint32(v4l2PixFmtRGB24)

	format := v4l2Format{Type: v4l2BufTypeVideoCapture}
	pix := (*v4l2PixFormat)(unsafe.Pointer(&format.fmt[0]))
	pix.Width = width
	pix.Height = height
	pix.Pixelformat = pixelFormat
	pix.Field = v4l2FieldAny

	if err := ioctl(fd, vidiocSFmt, unsafe.Pointer(&format)); err != nil {
		cleanup()
		return nil, fmt.Errorf("gocam: VIDIOC_S_FMT RGB24 failed: %w", err)
	}

	pixelFormat = pix.Pixelformat
	width = pix.Width
	height = pix.Height
	stride := int(pix.Bytesperline)

	if pixelFormat != v4l2PixFmtRGB24 {
		pixelFormat = v4l2PixFmtYUYV

		format = v4l2Format{Type: v4l2BufTypeVideoCapture}
		pix = (*v4l2PixFormat)(unsafe.Pointer(&format.fmt[0]))
		pix.Width = width
		pix.Height = height
		pix.Pixelformat = pixelFormat
		pix.Field = v4l2FieldAny

		if err := ioctl(fd, vidiocSFmt, unsafe.Pointer(&format)); err != nil {
			cleanup()
			return nil, fmt.Errorf("gocam: VIDIOC_S_FMT fallback YUYV failed: %w", err)
		}

		pixelFormat = pix.Pixelformat
		width = pix.Width
		height = pix.Height
		stride = int(pix.Bytesperline)
		if pixelFormat != v4l2PixFmtYUYV && pixelFormat != v4l2PixFmtRGB24 {
			cleanup()
			return nil, fmt.Errorf("gocam: unsupported pixel format 0x%x", pixelFormat)
		}
	}

	if stride == 0 {
		switch pixelFormat {
		case v4l2PixFmtRGB24:
			stride = int(width) * 3
		case v4l2PixFmtYUYV:
			stride = int(width) * 2
		}
	}

	req := v4l2RequestBuffers{
		Count:  4,
		Type:   v4l2BufTypeVideoCapture,
		Memory: v4l2MemoryMMap,
	}
	if err := ioctl(fd, vidiocReqbufs, unsafe.Pointer(&req)); err != nil {
		cleanup()
		return nil, fmt.Errorf("gocam: VIDIOC_REQBUFS failed: %w", err)
	}
	if req.Count < 2 {
		cleanup()
		return nil, fmt.Errorf("gocam: insufficient buffers: %d", req.Count)
	}

	buffers = make([]mappedBuffer, req.Count)

	for i := uint32(0); i < req.Count; i++ {
		buf := v4l2Buffer{
			Type:   v4l2BufTypeVideoCapture,
			Memory: v4l2MemoryMMap,
			Index:  i,
		}
		if err := ioctl(fd, vidiocQuerybuf, unsafe.Pointer(&buf)); err != nil {
			cleanup()
			return nil, fmt.Errorf("gocam: VIDIOC_QUERYBUF index %d failed: %w", i, err)
		}

		data, err := syscall.Mmap(fd, int64(buf.Offset), int(buf.Length), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("gocam: mmap buffer %d failed: %w", i, err)
		}

		buffers[i] = mappedBuffer{data: data, length: buf.Length}

		if err := ioctl(fd, vidiocQBuf, unsafe.Pointer(&buf)); err != nil {
			cleanup()
			return nil, fmt.Errorf("gocam: VIDIOC_QBUF index %d failed: %w", i, err)
		}
	}

	bufType := uint32(v4l2BufTypeVideoCapture)
	if err := ioctl(fd, vidiocStreamOn, unsafe.Pointer(&bufType)); err != nil {
		cleanup()
		return nil, fmt.Errorf("gocam: VIDIOC_STREAMON failed: %w", err)
	}
	streamStarted = true

	frameW := int(width)
	frameH := int(height)
	if frameW <= 0 || frameH <= 0 {
		cleanup()
		return nil, fmt.Errorf("gocam: invalid frame size %dx%d", frameW, frameH)
	}

	frames := make(chan Frame, 1)

	go func() {
		defer close(frames)
		defer cleanup()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			buf := v4l2Buffer{
				Type:   v4l2BufTypeVideoCapture,
				Memory: v4l2MemoryMMap,
			}

			if err := ioctl(fd, vidiocDQBuf, unsafe.Pointer(&buf)); err != nil {
				if errno, ok := err.(syscall.Errno); ok && (errno == syscall.EAGAIN || errno == syscall.EINTR) {
					time.Sleep(5 * time.Millisecond)
					continue
				}
				return
			}

			index := buf.Index
			if int(index) >= len(buffers) {
				_ = ioctl(fd, vidiocQBuf, unsafe.Pointer(&buf))
				continue
			}

			data := buffers[index].data
			sz := int(buf.Bytesused)
			if sz <= 0 || sz > len(data) {
				sz = len(data)
			}
			src := data[:sz]

			frameData := convertFrame(src, pixelFormat, frameW, frameH, stride)

			if err := ioctl(fd, vidiocQBuf, unsafe.Pointer(&buf)); err != nil {
				return
			}

			if frameData == nil {
				continue
			}

			frame := Frame{
				Data:   frameData,
				Width:  frameW,
				Height: frameH,
			}

			select {
			case frames <- frame:
			default:
				<-frames
				frames <- frame
			}
		}
	}()

	return frames, nil
}

func convertFrame(src []byte, pixFmt uint32, width, height, stride int) []byte {
	switch pixFmt {
	case v4l2PixFmtRGB24:
		rowBytes := width * 3
		if rowBytes <= 0 || len(src) < rowBytes {
			return nil
		}
		if height <= 0 {
			return nil
		}

		effectiveStride := stride
		if effectiveStride <= 0 {
			effectiveStride = rowBytes
		}
		if height > 0 && effectiveStride*height > len(src) {
			effectiveStride = len(src) / height
			if effectiveStride < rowBytes {
				return nil
			}
		}

		dst := make([]byte, rowBytes*height)
		for y := 0; y < height; y++ {
			start := y * effectiveStride
			end := start + rowBytes
			if end > len(src) {
				return nil
			}
			copy(dst[y*rowBytes:(y+1)*rowBytes], src[start:end])
		}
		return dst

	case v4l2PixFmtYUYV:
		rowBytes := width * 2
		if height <= 0 || rowBytes <= 0 {
			return nil
		}

		effectiveStride := stride
		if effectiveStride <= 0 {
			effectiveStride = rowBytes
		}
		if height > 0 && effectiveStride*height > len(src) {
			effectiveStride = len(src) / height
			if effectiveStride < rowBytes {
				return nil
			}
		}

		dst := make([]byte, width*height*3)
		for y := 0; y < height; y++ {
			inStart := y * effectiveStride
			inEnd := inStart + rowBytes
			if inEnd > len(src) {
				return nil
			}
			outStart := y * width * 3
			yuyvToRGBRow(dst[outStart:outStart+width*3], src[inStart:inEnd], width)
		}
		return dst

	default:
		return nil
	}
}

func yuyvToRGBRow(dst, src []byte, width int) {
	for x := 0; x < width; x += 2 {
		i := x * 2
		if i+3 >= len(src) {
			break
		}

		y0 := int(src[i])
		u := int(src[i+1]) - 128
		y1 := int(src[i+2])
		v := int(src[i+3]) - 128

		c0 := y0 - 16
		if c0 < 0 {
			c0 = 0
		}
		c1 := y1 - 16
		if c1 < 0 {
			c1 = 0
		}

		r0 := (298*c0 + 409*v + 128) >> 8
		g0 := (298*c0 - 100*u - 208*v + 128) >> 8
		b0 := (298*c0 + 516*u + 128) >> 8

		r1 := (298*c1 + 409*v + 128) >> 8
		g1 := (298*c1 - 100*u - 208*v + 128) >> 8
		b1 := (298*c1 + 516*u + 128) >> 8

		j := x * 3
		if j+5 >= len(dst) {
			break
		}

		dst[j] = clampToByte(r0)
		dst[j+1] = clampToByte(g0)
		dst[j+2] = clampToByte(b0)

		dst[j+3] = clampToByte(r1)
		dst[j+4] = clampToByte(g1)
		dst[j+5] = clampToByte(b1)
	}
}

func clampToByte(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

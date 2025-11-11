//go:build linux
// +build linux

package gocam

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	VIDIOC_S_FMT = 0xC0D05605

	V4L2_BUF_TYPE_VIDEO_CAPTURE = 1
	V4L2_PIX_FMT_RGB24          = 0x33424752 // 'RGB3' in little endian
)

type v4l2_pix_format struct {
	Width        uint32
	Height       uint32
	Pixelformat  uint32
	Field        uint32
	Bytesperline uint32
	Sizeimage    uint32
	Colorspace   uint32
	Priv         uint32
	Flags        uint32
	Ycbcr_enc    uint32
	Quantization uint32
	Xfer_func    uint32
}

type v4l2_format struct {
	Type uint32
	fmt  [200]byte // enough space for v4l2_pix_format
}

func StartStream(ctx context.Context) (<-chan Frame, error) {
	fd, err := syscall.Open("/dev/video0", syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("gocam: cannot open /dev/video0: %w", err)
	}

	// Prepare format struct
	var format v4l2_format
	format.Type = V4L2_BUF_TYPE_VIDEO_CAPTURE

	pix := (*v4l2_pix_format)(unsafe.Pointer(&format.fmt[0]))
	pix.Width = 640
	pix.Height = 480
	pix.Pixelformat = V4L2_PIX_FMT_RGB24
	pix.Field = 1 // V4L2_FIELD_NONE

	// Set format
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(VIDIOC_S_FMT), uintptr(unsafe.Pointer(&format)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("gocam: VIDIOC_S_FMT ioctl failed: %v", errno)
	}

	frameSize := int(pix.Width * pix.Height * 3) // RGB24

	ch := make(chan Frame, 1)

	go func() {
		defer syscall.Close(fd)
		defer close(ch)

		buf := make([]byte, frameSize)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := syscall.Read(fd, buf)
			if err != nil {
				if err == syscall.EAGAIN || err == syscall.EINTR {
					continue
				}
				return
			}
			if n < frameSize {
				continue
			}

			frameData := make([]byte, frameSize)
			copy(frameData, buf[:frameSize])

			select {
			case ch <- Frame{Data: frameData, Width: int(pix.Width), Height: int(pix.Height)}:
			default:
				// Если потребитель не успевает, старый кадр перезаписываем.
				<-ch
				ch <- Frame{Data: frameData, Width: int(pix.Width), Height: int(pix.Height)}
			}
		}
	}()

	return ch, nil
}

//go:build darwin
// +build darwin

package gocam

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc -fmodules
#cgo darwin LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework Foundation

#import <AVFoundation/AVFoundation.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <stdlib.h>
#import <string.h>

static AVCaptureSession *gSession;
static dispatch_queue_t gQueue;
static uint8_t *gFrameBuf;
static int gFrameWidth;
static int gFrameHeight;
static int gFrameReady;
static NSLock *gLock;

@interface GoFrameDelegate : NSObject<AVCaptureVideoDataOutputSampleBufferDelegate>
@end

@implementation GoFrameDelegate
- (void)captureOutput:(AVCaptureOutput *)output
 didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer
        fromConnection:(AVCaptureConnection *)connection
{
    CVImageBufferRef img = CMSampleBufferGetImageBuffer(sampleBuffer);
    if (!img) return;

    CVPixelBufferLockBaseAddress(img, kCVPixelBufferLock_ReadOnly);

    size_t w = CVPixelBufferGetWidth(img);
    size_t h = CVPixelBufferGetHeight(img);

    if (!CVPixelBufferIsPlanar(img) || CVPixelBufferGetPlaneCount(img) < 2) {
        CVPixelBufferUnlockBaseAddress(img, kCVPixelBufferLock_ReadOnly);
        return;
    }

    uint8_t *srcY  = (uint8_t *)CVPixelBufferGetBaseAddressOfPlane(img, 0);
    size_t strideY = CVPixelBufferGetBytesPerRowOfPlane(img, 0);

    uint8_t *srcUV  = (uint8_t *)CVPixelBufferGetBaseAddressOfPlane(img, 1);
    size_t strideUV = CVPixelBufferGetBytesPerRowOfPlane(img, 1);

    if (!srcY || !srcUV || strideY == 0 || strideUV == 0 || w == 0 || h == 0) {
        CVPixelBufferUnlockBaseAddress(img, kCVPixelBufferLock_ReadOnly);
        return;
    }

    [gLock lock];

    if (!gFrameBuf || gFrameWidth != (int)w || gFrameHeight != (int)h) {
        if (gFrameBuf) {
            free(gFrameBuf);
        }
        gFrameBuf = (uint8_t *)malloc(w * h * 3);
        gFrameWidth = (int)w;
        gFrameHeight = (int)h;
    }

    for (size_t y = 0; y < h; y++) {
        uint8_t *rowY  = srcY + y * strideY;
        uint8_t *rowUV = srcUV + (y / 2) * strideUV;

        for (size_t x = 0; x < w; x++) {
            uint8_t Y = rowY[x];
            uint8_t U = rowUV[(x / 2) * 2 + 0];
            uint8_t V = rowUV[(x / 2) * 2 + 1];

            int C = (int)Y - 16;
            int D = (int)U - 128;
            int E = (int)V - 128;
            if (C < 0) C = 0;

            int R = (298 * C + 409 * E + 128) >> 8;
            int G = (298 * C - 100 * D - 208 * E + 128) >> 8;
            int B = (298 * C + 516 * D + 128) >> 8;

            if (R < 0) R = 0; else if (R > 255) R = 255;
            if (G < 0) G = 0; else if (G > 255) G = 255;
            if (B < 0) B = 0; else if (B > 255) B = 255;

            size_t idx = (y * w + x) * 3;
            gFrameBuf[idx + 0] = (uint8_t)R;
            gFrameBuf[idx + 1] = (uint8_t)G;
            gFrameBuf[idx + 2] = (uint8_t)B;
        }
    }

    gFrameReady = 1;

    [gLock unlock];

    CVPixelBufferUnlockBaseAddress(img, kCVPixelBufferLock_ReadOnly);
}
@end

static GoFrameDelegate *gDelegate;

// StartCapture: 0 ok, <0 error
int StartCapture() {
    @autoreleasepool {
        gLock = [NSLock new];

        AVCaptureDevice *dev = [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo];
        if (!dev) return -1;

        NSError *err = nil;
        AVCaptureDeviceInput *input = [AVCaptureDeviceInput deviceInputWithDevice:dev error:&err];
        if (err || !input) return -2;

        AVCaptureSession *session = [[AVCaptureSession alloc] init];
        if (!session) return -3;

        [session beginConfiguration];
        if ([session canSetSessionPreset:AVCaptureSessionPreset640x480]) {
            session.sessionPreset = AVCaptureSessionPreset640x480;
        }

        if (![session canAddInput:input]) {
            return -4;
        }
        [session addInput:input];

        AVCaptureVideoDataOutput *out = [[AVCaptureVideoDataOutput alloc] init];
        NSDictionary *settings = @{
            (id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_420YpCbCr8BiPlanarFullRange)
        };
        out.videoSettings = settings;
        out.alwaysDiscardsLateVideoFrames = YES;

        gDelegate = [GoFrameDelegate new];
        gQueue = dispatch_queue_create("go.av.capture", DISPATCH_QUEUE_SERIAL);
        [out setSampleBufferDelegate:gDelegate queue:gQueue];

        if (![session canAddOutput:out]) {
            return -5;
        }
        [session addOutput:out];

        [session commitConfiguration];
        [session startRunning];

        gSession = session;
    }
    return 0;
}

void StopCapture() {
    @autoreleasepool {
        if (gSession) {
            [gSession stopRunning];
            gSession = nil;
        }
        if (gFrameBuf) {
            free(gFrameBuf);
            gFrameBuf = NULL;
        }
        gFrameWidth = 0;
        gFrameHeight = 0;
        gFrameReady = 0;
        gDelegate = nil;
        gQueue = nil;
        gLock = nil;
    }
}

// GetFrame: 0 ok, -1 no new frame
int GetFrame(uint8_t **buf, int *w, int *h) {
    if (!gFrameBuf || !gLock) {
        return -1;
    }

    [gLock lock];

    if (!gFrameReady) {
        [gLock unlock];
        return -1;
    }

    *buf = gFrameBuf;
    *w = gFrameWidth;
    *h = gFrameHeight;
    gFrameReady = 0; // mark as consumed

    [gLock unlock];

    return 0;
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"
)

// StartStream запускает захват с камеры и возвращает канал с RGB кадрами.
// Управление жизненным циклом через ctx: при отмене контекста захват останавливается.
func StartStream(ctx context.Context) (<-chan Frame, error) {
	rc := C.StartCapture()
	if rc != 0 {
		return nil, fmt.Errorf("cannot start capture, rc=%d", int(rc))
	}

	frames := make(chan Frame, 1)

	go func() {
		defer close(frames)
		defer C.StopCapture()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var cbuf *C.uchar
			var cw, ch C.int

			if C.GetFrame(&cbuf, &cw, &ch) != 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			w := int(cw)
			h := int(ch)
			if w <= 0 || h <= 0 || cbuf == nil {
				continue
			}

			size := w * h * 3
			data := C.GoBytes(unsafe.Pointer(cbuf), C.int(size))

			frame := Frame{
				Data:   data,
				Width:  w,
				Height: h,
			}

			select {
			case frames <- frame:
			default:
				// Если потребитель не успевает, старый кадр перезаписываем.
				<-frames
				frames <- frame
			}
		}
	}()

	return frames, nil
}

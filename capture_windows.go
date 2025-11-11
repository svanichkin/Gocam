//go:build windows
// +build windows

package gocam

/*
#cgo windows CFLAGS: -DUNICODE -D_UNICODE
#cgo windows LDFLAGS: -lole32 -lmfplat -lmf -lmfreadwrite -luuid

#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mfreadwrite.h>
#include <mferror.h>
#include <stdio.h>

static IMFSourceReader *gReader = NULL;
static CRITICAL_SECTION gLock;
static int gLockInit = 0;

static BYTE *gBuf = NULL;
static LONG gW = 0;
static LONG gH = 0;
static int gReady = 0;

static void gcam_init_lock() {
	if (!gLockInit) {
		InitializeCriticalSection(&gLock);
		gLockInit = 1;
	}
}

static void gcam_free_buf() {
	if (gBuf) {
		free(gBuf);
		gBuf = NULL;
	}
	gW = 0;
	gH = 0;
	gReady = 0;
}

// StartCapture инициализирует Media Foundation, выбирает первую доступную камеру
// и создаёт IMFSourceReader, настроенный на RGB24.
HRESULT StartCapture() {
	HRESULT hr;

	// COM + MF
	hr = CoInitializeEx(NULL, COINIT_MULTITHREADED);
	if (FAILED(hr) && hr != RPC_E_CHANGED_MODE) {
		return hr;
	}

	hr = MFStartup(MF_VERSION, MFSTARTUP_FULL);
	if (FAILED(hr)) {
		CoUninitialize();
		return hr;
	}

	gcam_init_lock();

	IMFAttributes *attr = NULL;
	IMFActivate **devices = NULL;
	UINT32 count = 0;

	hr = MFCreateAttributes(&attr, 1);
	if (FAILED(hr)) goto fail;

	hr = attr->SetGUID(MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE,
	                   MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_GUID);
	if (FAILED(hr)) goto fail;

	hr = MFEnumDeviceSources(attr, &devices, &count);
	if (FAILED(hr) || count == 0) {
		hr = E_FAIL;
		goto fail;
	}

	IMFMediaSource *source = NULL;
	hr = devices[0]->ActivateObject(&IID_IMFMediaSource, (void**)&source);
	if (FAILED(hr)) goto fail;

	hr = MFCreateSourceReaderFromMediaSource(source, NULL, &gReader);
	if (FAILED(hr)) {
		source->Release();
		goto fail;
	}

	// Настраиваем желаемый формат RGB24
	IMFMediaType *type = NULL;
	hr = MFCreateMediaType(&type);
	if (FAILED(hr)) goto fail;

	hr = type->SetGUID(&MF_MT_MAJOR_TYPE, &MFMediaType_Video);
	if (FAILED(hr)) goto fail;

	hr = type->SetGUID(&MF_MT_SUBTYPE, &MFVideoFormat_RGB24);
	if (FAILED(hr)) goto fail;

	hr = gReader->SetCurrentMediaType(MF_SOURCE_READER_FIRST_VIDEO_STREAM, NULL, type);
	if (FAILED(hr)) goto fail;

	// Попробуем вытащить размер кадра, если есть
	UINT32 w = 0, h = 0;
	hr = MFGetAttributeSize(type, &MF_MT_FRAME_SIZE, &w, &h);
	if (SUCCEEDED(hr) && w > 0 && h > 0) {
		gW = (LONG)w;
		gH = (LONG)h;
	} else {
		// fallback, если нет инфы
		gW = 640;
		gH = 480;
	}

	type->Release();
	source->Release();
	attr->Release();

	for (UINT32 i = 0; i < count; i++) {
		devices[i]->Release();
	}
	CoTaskMemFree(devices);

	return S_OK;

fail:
	if (gReader) {
		gReader->Release();
		gReader = NULL;
	}

	if (devices) {
		for (UINT32 i = 0; i < count; i++) {
			if (devices[i]) devices[i]->Release();
		}
		CoTaskMemFree(devices);
	}
	if (attr) attr->Release();

	MFShutdown();
	CoUninitialize();

	return hr;
}

// GetFrame: 0 ok, -1 no new frame
int GetFrame(unsigned char **buf, int *w, int *h) {
	if (!gReader || !gLockInit) {
		return -1;
	}

	HRESULT hr;
	DWORD flags = 0;
	IMFSample *sample = NULL;

	hr = gReader->ReadSample(
		MF_SOURCE_READER_FIRST_VIDEO_STREAM,
		0,
		NULL,
		&flags,
		NULL,
		&sample);
	if (FAILED(hr) || !sample) {
		return -1;
	}

	IMFMediaBuffer *mbuf = NULL;
	hr = sample->ConvertToContiguousBuffer(&mbuf);
	if (FAILED(hr) || !mbuf) {
		if (sample) sample->Release();
		return -1;
	}

	BYTE *data = NULL;
	DWORD len = 0;
	hr = mbuf->Lock(&data, NULL, &len);
	if (FAILED(hr) || !data || len == 0) {
		mbuf->Release();
		sample->Release();
		return -1;
	}

	int frameSize = (int)(gW * gH * 3);
	if (frameSize <= 0 || (DWORD)frameSize > len) {
		// Если размер неизвестен/некорректен, пытаемся угадать из len
		if (len % 3 == 0) {
			frameSize = (int)len;
		} else {
			mbuf->Unlock();
			mbuf->Release();
			sample->Release();
			return -1;
		}
	}

	EnterCriticalSection(&gLock);

	if (!gBuf || frameSize > (int)(gW * gH * 3)) {
		gcam_free_buf();
		gBuf = (BYTE*)malloc(frameSize);
	}

	if (gBuf) {
		memcpy(gBuf, data, frameSize);
		gReady = 1;
		*buf = gBuf;
		*w = gW;
		*h = gH;
	} else {
		gReady = 0;
		*buf = NULL;
	}

	LeaveCriticalSection(&gLock);

	mbuf->Unlock();
	mbuf->Release();
	sample->Release();

	return gReady ? 0 : -1;
}

void StopCapture() {
	if (gLockInit) {
		EnterCriticalSection(&gLock);
	}

	if (gReader) {
		gReader->Release();
		gReader = NULL;
	}

	gcam_free_buf();

	if (gLockInit) {
		LeaveCriticalSection(&gLock);
		DeleteCriticalSection(&gLock);
		gLockInit = 0;
	}

	MFShutdown();
	CoUninitialize();
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"
)

// StartStream запускает захват с камеры на Windows через Media Foundation
// и возвращает канал с RGB кадрами (Width x Height x 3).
func StartStream(ctx context.Context) (<-chan Frame, error) {
	hr := C.StartCapture()
	if hr != 0 {
		return nil, fmt.Errorf("gocam: cannot start capture, hr=0x%x", uint32(hr))
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

			if C.GetFrame(&cbuf, &cw, &ch) != 0 || cbuf == nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			w := int(cw)
			h := int(ch)
			if w <= 0 || h <= 0 {
				time.Sleep(5 * time.Millisecond)
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

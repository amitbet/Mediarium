package speed

import "testing"

func TestUploadOffIsNotALimit(t *testing.T) {
	t.Cleanup(func() { SetUpload(0); SetUploadOff(false) })
	if UploadOff() {
		t.Fatal("uploading starts on")
	}
	SetUploadOff(true)
	if !UploadOff() || CurrentUpload() != 0 {
		t.Fatalf("off %v, limit %d: switching off sets no limit", UploadOff(), CurrentUpload())
	}
	SetUpload(100 << 10)
	if !UploadOff() || CurrentUpload() != 100<<10 {
		t.Fatalf("a limit and the switch are separate: off %v, limit %d", UploadOff(), CurrentUpload())
	}
	SetUploadOff(false)
	if UploadOff() {
		t.Fatal("switched on again")
	}
}

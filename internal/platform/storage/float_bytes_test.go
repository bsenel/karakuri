package storage

import (
	"bytes"
	"testing"
)

// Stored embeddings are read back by whatever wrote them before, so the layout
// is pinned: four little-endian IEEE 754 bytes per value, in order.
func TestFloat32SliceToBytesIsLittleEndianIEEE754(t *testing.T) {
	got := float32SliceToBytes([]float32{1, -2.5, 0})
	want := []byte{
		0x00, 0x00, 0x80, 0x3f, // 1.0
		0x00, 0x00, 0x20, 0xc0, // -2.5
		0x00, 0x00, 0x00, 0x00, // 0
	}
	if !bytes.Equal(got, want) {
		t.Errorf("float32SliceToBytes = % x, want % x", got, want)
	}
	if got := float32SliceToBytes(nil); len(got) != 0 {
		t.Errorf("float32SliceToBytes(nil) = % x, want empty", got)
	}
}

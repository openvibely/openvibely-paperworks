package equation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The independently retained reference was emitted by unchanged equation.cjs
// using Apple Color Emoji. Expected bytes are never inputs to the renderer.
func TestPinnedEmojiEndToEnd(t *testing.T) {
	filename := os.Getenv("PAPERWORKS_EMOJI_FONT")
	if filename == "" {
		filename = "/System/Library/Fonts/Apple Color Emoji.ttc"
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Skip("pinned font not installed:", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "435ee0b75b26d671e089ddad38640473cc31febc4dbdfae27843a0a000d7ff62" {
		t.Skip("different font version; exact pinned comparison not applicable")
	}
	t.Setenv("PAPERWORKS_EMOJI_FONT", filename)
	output := filepath.Join(t.TempDir(), "emoji.png")
	result, err := Write(context.Background(), `\unicode{x1F600}`, output, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if result.Width != 90 || result.Height != 125 {
		t.Fatalf("dimensions %+v", result)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "44184e70c7605d8c461ecb2c8c82462dc01dae9b19869c1a8fe79dcd28911d93" {
		t.Fatalf("complete reference PNG differs: %s", got)
	}
}

func TestEmojiMissingFontLeavesNoOutput(t *testing.T) {
	t.Setenv("PAPERWORKS_EMOJI_FONT", filepath.Join(t.TempDir(), "absent.ttc"))
	destination := filepath.Join(t.TempDir(), "emoji.png")
	if _, err := Write(context.Background(), `\unicode{x1F600}`, destination, Defaults()); err == nil {
		t.Fatal("missing font accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("unexpected output: %v", err)
	}
}

func TestQuartzBitmapInterpolation(t *testing.T) {
	// Independent CoreGraphics alpha ramps distinguish quantized weights,
	// separate-sample truncation, and the otherwise ambiguous midpoint.
	for _, c := range []struct{ a, b, f, want float64 }{
		{0, 255, .15, 15}, {0, 255, .25, 31}, {0, 255, .35, 63}, {0, 255, .45, 127},
		{0, 255, .55, 128}, {0, 255, .65, 192}, {0, 255, .75, 224}, {0, 255, .85, 240},
		{127, 128, .15, 128}, {127, 128, .85, 127}, {1, 2, .5, 2}, {128, 127, .5, 127},
	} {
		if got := bitmapInterpolate(c.a, c.b, c.f); got != c.want {
			t.Fatalf("%+v got %v", c, got)
		}
	}
}

package zone

import (
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
)

// A step down leaves a quarter of an image's streamed bytes, each streamed
// part rounded up to 64 KiB; an image at 1/8 or with doNotResize can't step.
func TestStepSaves(t *testing.T) {
	img := func(streamed int64, mip string, locked bool) Image {
		return Image{Packed: Packed{Streamed: streamed}, MipBase: mip, Locked: locked}
	}
	for _, c := range []struct {
		im   Image
		want int64
	}{
		{img(4<<20, "1/1", false), 3 << 20},
		{img(1<<20, "1/2", false), 768 << 10},
		{img(256<<10, "1/4", false), 192 << 10},
		{img(64<<10, "1/1", false), 0},                             // a quarter rounds back up to one part
		{img(1000, "1/1", false), 0},                               // would grow to a whole part
		{img(4<<20, "1/8", false), 0},                              // the smallest step
		{img(4<<20, "1/1", true), 0},                               // doNotResize
		{img((1<<20)+4, "1/1", false), (1 << 20) + 4 - 5*(64<<10)}, // a quarter of it spills into a fifth part
	} {
		if got := c.im.StepSaves(); got != c.want {
			t.Errorf("%d bytes at %s (locked %v): saves %d, want %d", c.im.Streamed, c.im.MipBase, c.im.Locked, got, c.want)
		}
	}
}

// Who defines a packed image: a GDT of yours (its mipBase, inherited or
// blank, and doNotResize read), a stock GDT, or none.
func TestClassifyImage(t *testing.T) {
	_, w := gdtRoot(t, map[string]string{
		"source_data/mine.gdt": "{\n" + gdtAsset("plain", "image") + gdtAsset("base_img", "image", "mipBase", "1/2") +
			"\t\"half\" [ \"base_img\" ]\n\t{\n\t}\n" + gdtAsset("locked", "image", "doNotResize", "1") +
			gdtAsset("unlocked", "image", "doNotResize", "0", "mipBase", "1/4") + gdtAsset("blank", "image", "mipBase", "") +
			gdtAsset("dup", "image") + "}\n",
		"source_data/other.gdt": "{\n" + gdtAsset("dup", "image") + "}\n",
		"source_data/stock.gdt": "{\n" + gdtAsset("stock_c", "image") + "}\n",
	})
	packed := func(name string) Packed { return Packed{ID: asset.ID{Type: "image", Name: name}, Streamed: 1 << 20} }
	for _, c := range []struct {
		name    string
		owner   Owner
		mip     string
		locked  bool
		also    int
		gdtFile string
	}{
		{"plain", Yours, "1/1", false, 0, "source_data/mine.gdt"},
		{"half", Yours, "1/2", false, 0, "source_data/mine.gdt"},
		{"locked", Yours, "1/1", true, 0, "source_data/mine.gdt"},
		{"unlocked", Yours, "1/4", false, 0, "source_data/mine.gdt"},
		{"blank", Yours, "1/1", false, 0, "source_data/mine.gdt"},
		{"dup", Yours, "1/1", false, 1, ""},
		{"stock_c", Stock, "", false, 0, ""},
		{"$white", NoGDT, "", false, 0, ""},
	} {
		im, owner := ClassifyImage(w, packed(c.name))
		if owner != c.owner || im.MipBase != c.mip || im.Locked != c.locked || im.Also != c.also {
			t.Errorf("%s: %s %+v", c.name, owner, im)
		}
		if c.gdtFile != "" && im.Loc.File != c.gdtFile {
			t.Errorf("%s: defined in %q", c.name, im.Loc.File)
		}
		if owner == Yours && im.Name != c.name {
			t.Errorf("%s: carries %+v", c.name, im.Packed)
		}
	}
}

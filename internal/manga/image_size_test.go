//go:build outdated

package manga

import (
	_ "image/jpeg" // Register JPEG format
	_ "image/png"  // Register PNG format
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestGetImageNaturalSize(t *testing.T) {
	// Test the function
	width, height, err := getImageNaturalSize("")
	if err != nil {
		t.Fatal(err)
	}

	spew.Dump(width, height)
}

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var addr = "localhost:5000"
var imgDst = addr + "/ref"

func mustParse(ref string) name.Reference {
	r, err := name.ParseReference(ref, name.WeakValidation)
	if err != nil {
		panic(err)
	}
	return r
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "USAGE: ref <from> <at>")
		os.Exit(1)
	}

	from := mustParse(os.Args[1])
	at := mustParse(os.Args[2])
	fmt.Printf("Creating a reference to image: %s at: %s\n", os.Args[1], os.Args[2])
	fromImg, _ := remote.Image(from)

	referencingImage, _ := mutate.Reference(fromImg, fromImg)

	if err := remote.Write(at, referencingImage); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type staticLayer struct {
	b  []byte
	mt types.MediaType
}

func (l *staticLayer) Digest() (v1.Hash, error) {
	h, _, err := v1.SHA256(bytes.NewReader(l.b))
	return h, err
}

// DiffID returns the Hash of the uncompressed layer.
func (l *staticLayer) DiffID() (v1.Hash, error) {
	h, _, err := v1.SHA256(bytes.NewReader(l.b))
	return h, err
}

// Compressed returns an io.ReadCloser for the compressed layer contents.
func (l *staticLayer) Compressed() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(l.b)), nil
}

// Uncompressed returns an io.ReadCloser for the uncompressed layer contents.
func (l *staticLayer) Uncompressed() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(l.b)), nil
}

// Size returns the compressed size of the Layer.
func (l *staticLayer) Size() (int64, error) {
	return int64(len(l.b)), nil
}

// MediaType returns the media type of the Layer.
func (l *staticLayer) MediaType() (types.MediaType, error) {
	return l.mt, nil
}

package tests

import (
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/filesystem"
	"github.com/huwenlong92/sdkit/pkg/filesystem/core"
)

func Example_upload() {
	fs, _ := filesystem.NewFromPolicy(core.StoragePolicy{Driver: "local", LocalDir: ""})

	data := "hello world"
	info := core.FileInfo{
		Name: "example.txt",
		Path: "example.txt",
		Size: int64(len(data)),
	}

	fs.Put(core.NewFileStream(strings.NewReader(data), info))
	// Output:
}

func Example_namer() {
	namer := &core.Namer{
		DirRule:  "{date}/{randomkey8}",
		FileRule: "{uuid}{ext}",
	}
	path := namer.Generate("photo.jpg")
	fmt.Println(len(path) > 20)
	// Output:
	// true
}

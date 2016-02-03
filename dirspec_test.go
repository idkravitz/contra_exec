package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kravitz/tram_exec/tram-commons/util"
)

// FTS structure description shortname
type FTSS map[string]interface{}

func generateUniqueName(basename string) (string, error) {
	u := make([]byte, 16, 16)
	_, err := rand.Read(u)
	if err != nil {
		return "", err
	}
	return basename + hex.EncodeToString(u), nil
}

func MakeTestDir(basename string) (string, error) {
	projRoot := os.Getenv("GOPATH")
	if len(projRoot) == 0 {
		return "", errors.New("GOPATH envvar should be set")
	}

	tmpRoot := filepath.Join(projRoot, "temp")
	err := os.Mkdir(tmpRoot, 0744)
	if err != nil && !os.IsExist(err) {
		return "", err
	}

	folderCreated := false
	var newFullPath string
	for !folderCreated {
		newFolderName, err := generateUniqueName(basename)
		if err != nil {
			return "", err
		}
		newFullPath = filepath.Join(tmpRoot, newFolderName)
		err = os.Mkdir(newFullPath, 0744)
		if err != nil && !os.IsExist(err) {
			return "", err
		}
		folderCreated = err == nil
	}

	return newFullPath, nil
}

func filePutString(directory, filename, content string) error {
	fullPath := filepath.Join(directory, filename)
	fh, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.WriteString(content)
	if err != nil {
		return err
	}
	return nil
}

func hasKey(m map[string]*fileTreeState, key string) bool {
	_, has := m[key]
	return has
}

func simpleSetup(t *testing.T) (targetDirname string) {
	targetDirname, err := MakeTestDir("test_filetreespec_")
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"file_A.txt", "file_B.txt", "file_C.txt"}
	for _, fileName := range files {
		err := filePutString(targetDirname, fileName, fileName)
		if err != nil {
			t.Fatal(err)
		}
	}
	return targetDirname
}

func fileTreeStateHasStrucureLike(fts *fileTreeState, ftss FTSS) bool {
	estLen := 0
	for name, nest := range ftss {
		if ftsChild, ok := fts.Children[name]; ok {
			if nest != nil {
				if !fileTreeStateHasStrucureLike(ftsChild, nest.(FTSS)) {
					return false
				}
			}
		} else {
			return false
		}
		estLen = estLen + 1
	}
	if len(fts.Children) != estLen {
		return false
	}
	return true
}

func TestFindChangesSimple(t *testing.T) {
	targetDirname := simpleSetup(t)

	ftsBefore, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	if !fileTreeStateHasStrucureLike(ftsBefore, FTSS{
		"file_A.txt": nil,
		"file_B.txt": nil,
		"file_C.txt": nil,
	}) {
		t.Fatal("Simple test setup fails structure test")
	}

	// Add one new file
	err = filePutString(targetDirname, "file_New.txt", "The new file!")
	if err != nil {
		t.Fatal(err)
	}
	// Change one file
	err = filePutString(targetDirname, "file_A.txt", "This file is changed")
	if err != nil {
		t.Fatal(err)
	}
	// Remove one file
	err = os.Remove(filepath.Join(targetDirname, "file_B.txt"))
	if err != nil {
		t.Fatal(err)
	}

	ftsAfter, err := getFileTreeState(targetDirname)

	ftsDiff := findFileTreeStateChanges(ftsBefore, ftsAfter)
	if ftsDiff == nil {
		t.Fatal("No changes found!")
	}

	if !fileTreeStateHasStrucureLike(ftsDiff, FTSS{
		"file_New.txt": nil,
		"file_A.txt":   nil,
	}) {
		t.Fatal("Expected two files diff in:", util.Qjson(ftsDiff))
	}

	// cleanup
	os.RemoveAll(targetDirname)
}

func idTest(directory, message string, t *testing.T) {
	ftsBefore, err := getFileTreeState(directory)
	if err != nil {
		t.Fatal(err)
	}
	ftsAfter, err := getFileTreeState(directory)
	if err != nil {
		t.Fatal(err)
	}
	ftsDiff := findFileTreeStateChanges(ftsBefore, ftsAfter)
	if ftsDiff != nil {
		t.Fatal(message)
	}
}

func TestFindChangesId(t *testing.T) {
	targetDirname, err := MakeTestDir("test_filetreespec_")
	if err != nil {
		t.Fatal(err)
	}
	idTest(targetDirname, "Empty dir should be unchanged", t)
	deepDir := filepath.Join(targetDirname, "A", "B", "C")
	os.MkdirAll(deepDir, 0700)
	idTest(targetDirname, "Deep empty dirs should be unchanged", t)
	filePutString(deepDir, "filey.txt", "I'm a small filey")
	idTest(targetDirname, "Deep empty dirs with one file inside should be unchanged", t)
	os.RemoveAll(targetDirname)
}

func TestFindChangesAdvanced(t *testing.T) {
	targetDirname := simpleSetup(t)

	os.Mkdir(filepath.Join(targetDirname, "Dir_A"), 0700)
	filePutString(filepath.Join(targetDirname, "Dir_A"), "file", "sample") // this file won't change
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_B"), 0700)         // this would be empty
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_C"), 0700)         // this would have one changed file, one new and one unchanged
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_A", "sample")
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_B", "sample")

	ftsBefore, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_B", "changed")
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_C", "new")
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_D"), 0700) // new empty dir
	ftsAfter, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	ftss := FTSS{
		"Dir_A": FTSS{
			"Dir_D": nil,
			"Dir_C": FTSS{
				"file_B": nil,
				"file_C": nil,
			},
		},
	}
	ftsDiff := findFileTreeStateChanges(ftsBefore, ftsAfter)
	if !(ftsDiff != nil && fileTreeStateHasStrucureLike(ftsDiff, ftss)) {
		t.Fatal("Advanced test failed with diff:", util.Qjson(ftsDiff))
	}
	os.RemoveAll(targetDirname)
}

func _TestPackTree(t *testing.T) {
	// This test fails on windows, may need to avoid tar or smth like that
	// As one simple idea I can use 7z compresser
	targetDirname := simpleSetup(t)

	os.Mkdir(filepath.Join(targetDirname, "Dir_A"), 0700)
	filePutString(filepath.Join(targetDirname, "Dir_A"), "file", "sample") // this file won't change
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_B"), 0700)         // this would be empty
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_C"), 0700)         // this would have one changed file, one new and one unchanged
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_A", "sample")
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_B", "sample")

	ftsBefore, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_B", "changed")
	filePutString(filepath.Join(targetDirname, "Dir_A", "Dir_C"), "file_C", "new")
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_D"), 0700) // new empty dir
	ftsAfter, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	ftsDiff := findFileTreeStateChanges(ftsBefore, ftsAfter)
	// expected structure
	ftss := FTSS{
		"Dir_A": FTSS{
			"Dir_D": nil,
			"Dir_C": FTSS{
				"file_B": nil,
				"file_C": nil,
			},
		},
	}
	if !fileTreeStateHasStrucureLike(ftsDiff, ftss) {
		t.Fatal("Diff has wrong structure")
	}

	outDir, err := MakeTestDir("output_filetreespec_")
	if err != nil {
		t.Fatal(err)
	}

	err = packTree(filepath.Dir(targetDirname), outDir, "output.tar.gz", ftsDiff)
	if err != nil {
		t.Fatal(err)
	}

	midDir, err := MakeTestDir("unpack_filetreespec_")
	unpackData(midDir, outDir, "output.tar.gz")

	dh, err := os.Open(midDir)
	if err != nil {
		t.Fatal(err)
	}
	subDirs, err := dh.Readdirnames(1)
	if err != nil {
		t.Fatal(err)
	}
	testDir := filepath.Join(midDir, subDirs[0])
	unpackState, err := getFileTreeState(testDir)
	if err != nil {
		t.Fatal(err)
	}

	if !fileTreeStateHasStrucureLike(unpackState, ftss) {
		t.Fatal("Should be the same after unpack")
	}

	os.RemoveAll(targetDirname)
	os.RemoveAll(outDir)
}

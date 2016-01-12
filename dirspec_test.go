package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/kravitz/tram_api/tram-commons/util"
)

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
	targetDirname, err := ioutil.TempDir("", "test_filetreespec_")
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

func TestFindChangesSimple(t *testing.T) {
	targetDirname := simpleSetup(t)

	ftsBefore, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
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

	if !(len(ftsDiff.Children) == 2 &&
		hasKey(ftsDiff.Children, "file_New.txt") &&
		hasKey(ftsDiff.Children, "file_A.txt")) {
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
	targetDirname, err := ioutil.TempDir("", "test_filetreespec_")
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
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_D"), 0700) // new empty string
	ftsAfter, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	ftsDiff := findFileTreeStateChanges(ftsBefore, ftsAfter)
	if !(ftsDiff != nil &&
		ftsDiff.Children != nil &&
		len(ftsDiff.Children) == 1 &&
		hasKey(ftsDiff.Children, "Dir_A") &&
		ftsDiff.Children["Dir_A"].Children != nil &&
		len(ftsDiff.Children["Dir_A"].Children) == 2 &&
		hasKey(ftsDiff.Children["Dir_A"].Children, "Dir_D") &&
		hasKey(ftsDiff.Children["Dir_A"].Children, "Dir_C") &&
		ftsDiff.Children["Dir_A"].Children["Dir_C"].Children != nil &&
		len(ftsDiff.Children["Dir_A"].Children["Dir_C"].Children) == 2 &&
		hasKey(ftsDiff.Children["Dir_A"].Children["Dir_C"].Children, "file_B") &&
		hasKey(ftsDiff.Children["Dir_A"].Children["Dir_C"].Children, "file_C")) {
		t.Fatal("Advanced test failed with diff:", util.Qjson(ftsDiff))
	}
	os.RemoveAll(targetDirname)
}

func TestPackTree(t *testing.T) {
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
	os.Mkdir(filepath.Join(targetDirname, "Dir_A", "Dir_D"), 0700) // new empty string
	ftsAfter, err := getFileTreeState(targetDirname)
	if err != nil {
		t.Fatal(err)
	}
	ftsDiff := findFileTreeStateChanges(ftsBefore, ftsAfter)

	outDir, err := ioutil.TempDir("", "output_filetreespec_")
	if err != nil {
		t.Fatal(err)
	}

	err = packTree(filepath.Dir(targetDirname), outDir, "output.tar.gz", ftsDiff)
	if err != nil {
		t.Fatal(err)
	}
}

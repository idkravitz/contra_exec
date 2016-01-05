package main

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/kravitz/tram_exec/tram-commons/util"
)

func TestFindChanges(t *testing.T) {
	dirName, err := ioutil.TempDir("", "tram_exec_test")
	if err != nil {
		t.Error("Failed to create temp dir:", err)
	}

	files := []string{"file_A.txt", "file_B.txt", "file_C.txt"}
	for _, filename := range files {
		fullFilename := path.Join(dirName, filename)
		fh, err := os.OpenFile(fullFilename, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			t.Error(err)
		}
		fh.WriteString(fullFilename)
	}

	ds1 := dirSpec{}
	err = fillDirSpec(dirName, ds1)
	if err != nil {
		t.Fail()
	}
	t.Error((util.Qjson(ds1)))

	// s := diveIntoData(".")

	os.RemoveAll(dirName)
	// t.Error(s)
}

// func TestMain(m *testing.M) {
// m.Run()
// }

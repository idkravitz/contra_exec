package main

import "testing"

func TestFindChanges(t *testing.T) {
	s := diveIntoData(".")
	t.Error(s)
}

// func TestMain(m *testing.M) {
// m.Run()
// }

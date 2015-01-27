//
package main

import (
	"fmt"

	"github.com/btcsuite/goleveldb/leveldb"
	"github.com/btcsuite/goleveldb/leveldb/opt"
)

type tst struct {
	key   int
	value string
}

var dataset = []tst{
	//var dataset = []struct { key int, value string } {
	{1, "one"},
	{2, "two"},
	{3, "three"},
	{4, "four"},
	{5, "five"},
}

func main() {

	ro := &opt.ReadOptions{}
	wo := &opt.WriteOptions{}
	opts := &opt.Options{}

	ldb, err := leveldb.OpenFile("dbfile", opts)
	if err != nil {
		fmt.Printf("db open failed %v\n", err)
		return
	}

	batch := new(leveldb.Batch)
	for _, datum := range dataset {
		key := fmt.Sprintf("%v", datum.key)
		batch.Put([]byte(key), []byte(datum.value))
	}
	err = ldb.Write(batch, wo)

	for _, datum := range dataset {
		key := fmt.Sprintf("%v", datum.key)
		data, err := ldb.Get([]byte(key), ro)

		if err != nil {
			fmt.Printf("db read failed %v\n", err)
		}

		if string(data) != datum.value {
			fmt.Printf("mismatched data from db key %v val %v db %v", key, datum.value, data)
		}
	}
	fmt.Printf("completed\n")
	ldb.Close()
}

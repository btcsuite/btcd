package util

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Rand32Int() (*[32]byte, error) {
	var random [32]byte
	l, err := rand.Read(random[:])
	if err != nil {
		return nil, err
	}
	if l != 32 {
		return nil, fmt.Errorf("randomization error")
	}

	return &random, nil
}

func Rand32IntForTest(ht *testing.T) [32]byte {
	random, err := Rand32Int()
	require.NoError(ht, err)
	return *random
}

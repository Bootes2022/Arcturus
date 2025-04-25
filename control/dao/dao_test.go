package dao

import (
	"fmt"
	"testing"
)

func TestUseToml(t *testing.T) {
	c := UseToml()
	fmt.Println(c)
}

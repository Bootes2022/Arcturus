package middleware

import (
	"fmt"
	"testing"
)

func TestUseToml(t *testing.T) {
	c := UseToml()
	fmt.Println(c)
}

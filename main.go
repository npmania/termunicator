package main

import (
	"fmt"

	"termunicator/internal/lib"
)

func main() {
	greeting := lib.Greet("termunicator")
	fmt.Println(greeting)
	fmt.Println("Welcome to termunicator - a TUI for unified chat!")
}

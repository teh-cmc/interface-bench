package main

import (
	"fmt"
	"time"

	"github.com/pkg/profile"
)

// -----------------------------------------------------------------------------

// Int provides an `int64` that implements the `Summable` interface.
type Int int64

// Sum simply adds two `Int`s.
func (i *Int) Sum(i2 Int) { *i += i2 }

type Summable interface {
	Sum(i Int)
}

// -----------------------------------------------------------------------------

const nbOps int = 1e8

func main() {
	defer profile.Start(profile.CPUProfile).Stop()

	var start time.Time
	var zero Int

	var iConcrete *Int = &zero
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iConcrete.Sum(Int(10))
	}
	_ = iConcrete
	fmt.Printf("[concrete]  computed %d sums in %v\n", nbOps, time.Now().Sub(start))

	var iInterface Summable = &zero
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iInterface.Sum(Int(10))
	}
	_ = iInterface
	fmt.Printf("[interface] computed %d sums in %v\n", nbOps, time.Now().Sub(start))
}

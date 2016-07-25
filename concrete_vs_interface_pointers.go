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
func (i *Int) Sum(i2 Int) *Int { *i += i2; return i }

type Summable interface {
	Sum(i Int) *Int
}

// -----------------------------------------------------------------------------

const nbOps int = 1e8

func main() {
	defer profile.Start(profile.CPUProfile).Stop()

	var start time.Time
	var zero Int

	var iConcreteP *Int = &zero
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iConcreteP = iConcreteP.Sum(Int(10))
	}
	_ = iConcreteP
	fmt.Printf("[concrete]  computed %d sums in %v\n", nbOps, time.Now().Sub(start))

	var iInterfaceP Summable = &zero
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iInterfaceP = iInterfaceP.Sum(Int(10))
	}
	_ = iInterfaceP
	fmt.Printf("[interface] computed %d sums in %v\n", nbOps, time.Now().Sub(start))
}

# Interface-Bench

A straightforward benchmark of the cost of Go's interfaces.

## The Code

It's all [there](./main.go).

For simplicity, here's a complete copy:  
```Go
package main

import (
	"fmt"
	"time"
)

// -----------------------------------------------------------------------------

// Int provides an `int64` that implements the `Summable` interface.
type Int int64

// Sum simply adds two `Int`s.
func (i Int) Sum(i2 Int) Int { return i + i2 }

type Summable interface {
	Sum(i Int) Int
}

// -----------------------------------------------------------------------------

const nbOps int = 1e8

func main() {
	var start time.Time

	var iConcrete Int
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iConcrete = iConcrete.Sum(Int(10))
	}
	_ = iConcrete
	fmt.Printf("[concrete]  computed %d sums in %v\n", nbOps, time.Now().Sub(start))

	var iInterface Summable = Int(0)
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iInterface = iInterface.Sum(Int(10))
	}
	_ = iInterface
	fmt.Printf("[interface] computed %d sums in %v\n", nbOps, time.Now().Sub(start))
}
```

Basically, we're comparing the cost of calling a method on a concrete type vs. an interface.

## The result

```
$ go run main.go
[concrete]  computed 100000000 sums in 41.966579ms
[interface] computed 100000000 sums in 2.799456753s
```

Making 100 million method calls on a concrete type is around 66 times faster than going through an interface.

## The conclusion

Theses things can be really slow, which may or may not be an issue depending on your use-case; just keep that in mind and everything shall be okay.

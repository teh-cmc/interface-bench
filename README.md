# Interface-Bench

Digging into some of Go's subtleties regarding the use of interfaces and the performance issues that might ensue.

## Round I: Method calls on concrete types vs. interfaces

Let's compare the performances of doing 100 million method calls on a concrete type (`int64`) vs. an interface.  
The code is [as follows](./concrete_vs_interface.go):  
```Go
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
func (i Int) Sum(i2 Int) Int { return i + i2 }

type Summable interface {
	Sum(i Int) Int
}

// -----------------------------------------------------------------------------

const nbOps int = 1e8

func main() {
	defer profile.Start(profile.CPUProfile).Stop()

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

Pretty straightforward stuff. The results look like these:  
```
$ go version
go version go1.6.2 darwin/amd64
$ sysctl -n machdep.cpu.brand_string
Intel(R) Core(TM) i7-4870HQ CPU @ 2.50GHz
$ go run main.go
[concrete]  computed 100000000 sums in 41.966579ms
[interface] computed 100000000 sums in 2.799456753s
```

At first glance, it would seem that going through the interface dispatch machinery comes with a terrifying 6500% slowdown... and, indeed, that's what I thought at first; until @twotwotwo [rightfully demonstrated how completely broken my benchmark actually was](https://github.com/teh-cmc/interface-bench/issues/1).

So, what is really going on here?  
For a concise answer, have a look at #1. If you're looking for the long version, just continue reading below.

### Why are the concrete calls this fast?

100 million calls to a `Sum` method in 42ms, that's a staggering ~2.4 billion sums per second. Anyway you look at it, that's _real_ fast.

There are two main reasons behind this speed.

**1. inlining**

Using the `-m` gcflag will show that the compiler is inlining the call to `iConcrete.Sum(Int(10))`:  
```
$ go run -gcflags '-m' main.go
# command-line-arguments
./main.go:16: can inline Int.Sum
./main.go:34: inlining call to Int.Sum
...(rest omitted)...
```

This obviously avoids a lot of copying. Running the same code with inlining disabled (via the `-l` gcflag) shows a 10x drop in performance:  
```
$ go run -gcflags '-l' main.go
[concrete]  computed 100000000 sums in 413.927641ms
```

**2. no escaping**

There are no pointers involved in this snippet, no variables escaping from the stack either; and with `Sum` being inlined, everything is actually happening within the same stack-frame: there is literally no work at all to be done by the memory allocator nor the garbage collector.

Go code can't go much faster than this.

### Why are the interface calls this slow?

100 million calls in 2800ms (~35.5 million per sec), on the other hand, seems particularly slow.

As @twotwotwo mentioned in #1, the reason for this slowdown stems from a change that shipped with the 1.4 release of the Go runtime:  
> The implementation of interface values has been modified. In earlier releases, the interface contained a word that was either a pointer or a one-word scalar value, depending on the type of the concrete object stored. This implementation was problematical for the garbage collector, so as of 1.4 interface values always hold a pointer. In running programs, most interface values were pointers anyway, so the effect is minimal, but programs that store integers (for example) in interfaces will see more allocations.

Because of this change, every time `iInterface.Sum(Int(10))` is called, `sizeof(iInterface)` bytes have to be allocated on the heap and the value of `iInterface` has to be copied to that new location.

This is obviously a huge amount of work; and indeed, a pprof trace show that most of the time is spent allocating bytes and copying values:  
```
  flat  flat%   sum%        cum   cum%
 740ms 28.24% 28.24%     1180ms 45.04%  runtime.mallocgc
 340ms 12.98% 41.22%     1520ms 58.02%  runtime.newobject
 270ms 10.31% 51.53%      270ms 10.31%  runtime.mach_semaphore_signal
 260ms  9.92% 61.45%     2490ms 95.04%  main.main
 220ms  8.40% 69.85%     2090ms 79.77%  runtime.convT2I
 180ms  6.87% 76.72%      180ms  6.87%  runtime.memmove
 150ms  5.73% 82.44%      330ms 12.60%  runtime.typedmemmove
 130ms  4.96% 87.40%      130ms  4.96%  main.(*Int).Sum
 100ms  3.82% 91.22%      100ms  3.82%  runtime.prefetchnta
  80ms  3.05% 94.27%       80ms  3.05%  runtime.(*mspan).sweep.func1
```

How can we fix this?

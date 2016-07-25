# Interface-Bench

Digging into some of Go's subtleties regarding the use of interfaces and the performance issues that might ensue.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Interface-Bench](#interface-bench)
  - [Round I: Method calls on concrete types vs. interfaces](#round-i-method-calls-on-concrete-types-vs-interfaces)
    - [Why are the concrete calls this fast?](#why-are-the-concrete-calls-this-fast)
    - [Why are the interface calls this slow?](#why-are-the-interface-calls-this-slow)
  - [Round II: Pointers](#round-ii-pointers)
    - [4x slower concrete calls](#4x-slower-concrete-calls)
    - [5x faster interface calls](#5x-faster-interface-calls)
  - [Round III: In-place](#round-iii-in-place)
    - [Wait... did the concrete calls just get slower?!](#wait-did-the-concrete-calls-just-get-slower)
    - [Interface calls are now as fast as concrete calls!](#interface-calls-are-now-as-fast-as-concrete-calls)
  - [Conclusion](#conclusion)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

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
$ go run concrete_vs_interface.go
[concrete]  computed 100000000 sums in 41.966579ms
[interface] computed 100000000 sums in 2.799456753s
```

At first glance, it would seem that going through the interface dispatch machinery comes with a terrifying 6500% slow-down... and, indeed, that's what I thought at first; until @twotwotwo [rightfully demonstrated how completely broken my benchmark actually was](https://github.com/teh-cmc/interface-bench/issues/1).

So, what is really going on here?  
For a concise answer, have a look at #1. If you're looking for the long version, you may continue reading below.

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

There are no pointers involved in this snippet, no variables escaping from the stack either; and with `Sum` being inlined, everything is literally happening within the same stack-frame: there is simply no work at all to be done by the memory allocator nor the garbage collector.

Go code can't go much faster than this.

### Why are the interface calls this slow?

100 million calls in 2800ms (~35.5 million per sec), on the other hand, seems particularly slow.

As @twotwotwo mentioned in #1, this slow-down stems from a change that shipped with the 1.4 release of the Go runtime:  
> The implementation of interface values has been modified. In earlier releases, the interface contained a word that was either a pointer or a one-word scalar value, depending on the type of the concrete object stored. This implementation was problematical for the garbage collector, so as of 1.4 interface values always hold a pointer. In running programs, most interface values were pointers anyway, so the effect is minimal, but programs that store integers (for example) in interfaces will see more allocations.

Because of this, every time `iInterface.Sum(Int(10))` returns a result, `sizeof(Int)` bytes have to be allocated on the heap and the value of the current variable has to be copied to that new location.

This obviously induces a huge amount of work; and, indeed, a pprof trace shows that most of the time is spent allocating bytes and copying values as part of the process of converting types to interfaces (i.e. `runtime.convT2I`):  
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

Note that GC/STW latencies are not even part of the equation here: if you try running this program with the GC disabled (`GOGC=off`), you should get the exact same results (with Go 1.6+ at least).

So, how can we fix this?

## Round II: Pointers

An idea that naturally comes to mind when trying to reduce copying is to use pointers.  
The code is [as follows](./concrete_vs_interface_pointers.go):  
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

	var iConcrete *Int = &zero
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iConcrete = iConcrete.Sum(Int(10))
	}
	_ = iConcrete
	fmt.Printf("[concrete]  computed %d sums in %v\n", nbOps, time.Now().Sub(start))

	var iInterface Summable = &zero
	start = time.Now()
	for i := 0; i < nbOps; i++ {
		iInterface = iInterface.Sum(Int(10))
	}
	_ = iInterface
	fmt.Printf("[interface] computed %d sums in %v\n", nbOps, time.Now().Sub(start))
}
```

The code is almost the same, except that `Sum` now applies to a pointer and returns that same pointer as a result.

The results look like these:  
```
$ go version
go version go1.6.2 darwin/amd64
$ sysctl -n machdep.cpu.brand_string
Intel(R) Core(TM) i7-4870HQ CPU @ 2.50GHz
$ go run concrete_vs_interface_pointers.go
[concrete]  computed 100000000 sums in 178.189757ms
[interface] computed 100000000 sums in 593.659837ms
```

### 4x slower concrete calls

The reason for this slow-down is simply the overhead of dereferencing the `iConcrete` pointer for each summation.

Not much more we can do here.

### 5x faster interface calls

The reason for this speed-up is that we've completely removed the need to allocate `sizeof(Int)` on the heap and copy values around every time we assign the return value of `Sum` to `iInterface`.

A quick look at the pprof trace will confirm our thoughts:  
```
  flat  flat%   sum%        cum   cum%
 280ms 56.00% 56.00%      280ms 56.00%  main.(*Int).Sum
 150ms 30.00% 86.00%      430ms 86.00%  main.main
  70ms 14.00%   100%       70ms 14.00%  runtime.usleep
     0     0%   100%      430ms 86.00%  runtime.goexit
     0     0%   100%      430ms 86.00%  runtime.main
     0     0%   100%       70ms 14.00%  runtime.mstart
     0     0%   100%       70ms 14.00%  runtime.mstart1
     0     0%   100%       70ms 14.00%  runtime.sysmon
```
There is effectively no trace of `runtime.mallocgc`, `runtime.convT2I` or anything else that's part of the process of converting types to interfaces (T2I) here.

This is still 3-4x slower than concrete calls though; can we make it even faster?

## Round III: In-place

Since we're now applying `Sum` to a pointer, we might as well not return anything.  
This will make some nice chaining patterns impossible but, on the other hand, should entirely remove the overhead of creating an interface every time we assign the return value of `Sum` to `iInterface`.  
The code is [as follows](./concrete_vs_interface_pointers_inplace.go):  
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
```

The results look like these:  
```
$ go version
go version go1.6.2 darwin/amd64
$ sysctl -n machdep.cpu.brand_string
Intel(R) Core(TM) i7-4870HQ CPU @ 2.50GHz
$ go run concrete_vs_interface_pointers_inplace.go
[concrete]  computed 100000000 sums in 215.192313ms
[interface] computed 100000000 sums in 222.486475ms
```

The overhead of going through an interface is now barely noticeable.. in fact, it's sometimes even faster than the concrete calls!

### Wait... did the concrete calls just get slower?!

_Yes, they did._  
Removing the return value of the `Sum` method noticeably made the concrete calls ~17% slower (going from 180ms on average to 210ms).

I haven't had the time to look into this, so I'm not sure about the exact cause for this slow-down; but I'm going to assume that the presence of the return value allows the compiler to do some tricky optimizations...  
I'll dig into this once I find the time; if you know what's going on, please open an issue!

### Interface calls are now as fast as concrete calls!

Finally, now that we've completely removed the need to build interfaces when assigning `Sum`'s return values; we've removed all the overhead we could remove.

In this configuration, using either concrete or interface calls has virtually the same cost (although, in reality, concrete calls can be made faster with the use of a return value).

## Conclusion

Technically, Go interfaces' method-dispatch machinery barely has any overhead compared to a simple method call on a concrete type.

In practice, due to the way interfaces are implemented, it's easy to stumble upon various more-or-less obvious pitfalls that can result in a lot of overhead, primarily caused by implicit allocations and copies.  
Sometimes, compiler optimizations will save you; and sometimes they won't.
